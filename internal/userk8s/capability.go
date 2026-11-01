package userk8s

import (
	"fmt"
	"slices"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

// EffectiveModel is a Model with one serving's narrowings applied. Consumers
// (projection, JS snapshot, gateway facts) read this instead of merging Model
// and Serving themselves.
type EffectiveModel struct {
	ContextWindow    int64
	MaxOutputTokens  int64
	InputModalities  []string
	OutputModalities []string
	Reasoning        *v1alpha1.ModelReasoning
	ToolCall         bool
	StructuredOutput bool
}

// effectiveModel applies one serving's overrides to its Model and enforces the
// narrowing rule: a provider may offer less than the canonical definition, not
// more. A provider that genuinely exceeds it means the Model is wrong and must
// be updated, so the catalog never accumulates contradictory capability
// claims.
func effectiveModel(model *v1alpha1.Model, serving *v1alpha1.ModelServing) (EffectiveModel, error) {
	if model == nil || serving == nil {
		return EffectiveModel{}, fmt.Errorf("effective model requires a Model and a ModelServing")
	}
	effective := EffectiveModel{
		ContextWindow:    model.Spec.ContextWindow,
		MaxOutputTokens:  model.Spec.MaxOutputTokens,
		InputModalities:  slices.Clone(model.Spec.InputModalities),
		OutputModalities: slices.Clone(model.Spec.OutputModalities),
		Reasoning:        cloneReasoning(model.Spec.Reasoning),
		ToolCall:         model.Spec.ToolCall,
		StructuredOutput: model.Spec.StructuredOutput,
	}
	overrides := serving.Spec.Overrides
	if overrides == nil {
		return effective, nil
	}
	if overrides.ContextWindow != nil {
		if *overrides.ContextWindow > model.Spec.ContextWindow {
			return EffectiveModel{}, fmt.Errorf("ModelServing %q override context_window %d exceeds Model %q's %d", serving.Name, *overrides.ContextWindow, model.Name, model.Spec.ContextWindow)
		}
		effective.ContextWindow = *overrides.ContextWindow
	}
	if overrides.MaxOutputTokens != nil {
		if *overrides.MaxOutputTokens > model.Spec.MaxOutputTokens {
			return EffectiveModel{}, fmt.Errorf("ModelServing %q override max_output_tokens %d exceeds Model %q's %d", serving.Name, *overrides.MaxOutputTokens, model.Name, model.Spec.MaxOutputTokens)
		}
		effective.MaxOutputTokens = *overrides.MaxOutputTokens
	}
	if overrides.InputModalities != nil {
		if err := narrowModalities("input", model.Name, serving.Name, model.Spec.InputModalities, overrides.InputModalities); err != nil {
			return EffectiveModel{}, err
		}
		effective.InputModalities = slices.Clone(overrides.InputModalities)
	}
	if overrides.OutputModalities != nil {
		if err := narrowModalities("output", model.Name, serving.Name, model.Spec.OutputModalities, overrides.OutputModalities); err != nil {
			return EffectiveModel{}, err
		}
		effective.OutputModalities = slices.Clone(overrides.OutputModalities)
	}
	if overrides.Reasoning != nil {
		if err := narrowReasoning(model, serving, overrides.Reasoning); err != nil {
			return EffectiveModel{}, err
		}
		effective.Reasoning = mergeReasoning(model.Spec.Reasoning, overrides.Reasoning)
		if effective.Reasoning != nil && effective.Reasoning.Budget != nil {
			if err := validateReasoningBudget("ModelServing", serving.Name, effective.Reasoning.Budget); err != nil {
				return EffectiveModel{}, err
			}
		}
	}
	if overrides.ToolCall != nil {
		if *overrides.ToolCall && !model.Spec.ToolCall {
			return EffectiveModel{}, fmt.Errorf("ModelServing %q enables tool_call, which Model %q does not define", serving.Name, model.Name)
		}
		effective.ToolCall = *overrides.ToolCall
	}
	if overrides.StructuredOutput != nil {
		if *overrides.StructuredOutput && !model.Spec.StructuredOutput {
			return EffectiveModel{}, fmt.Errorf("ModelServing %q enables structured_output, which Model %q does not define", serving.Name, model.Name)
		}
		effective.StructuredOutput = *overrides.StructuredOutput
	}
	return effective, nil
}

func narrowModalities(kind, modelName, servingName string, canonical, narrowed []string) error {
	for _, modality := range narrowed {
		if !slices.Contains(canonical, modality) {
			return fmt.Errorf("ModelServing %q %s modality %q is not part of Model %q's %v", servingName, kind, modality, modelName, canonical)
		}
	}
	return nil
}

func narrowReasoning(model *v1alpha1.Model, serving *v1alpha1.ModelServing, narrowed *v1alpha1.ModelReasoning) error {
	canonical := model.Spec.Reasoning
	if narrowed.Supported && (canonical == nil || !canonical.Supported) {
		return fmt.Errorf("ModelServing %q enables reasoning, which Model %q does not define", serving.Name, model.Name)
	}
	if narrowed.Toggle && (canonical == nil || !canonical.Toggle) {
		return fmt.Errorf("ModelServing %q enables the reasoning toggle, which Model %q does not define", serving.Name, model.Name)
	}
	if canonical == nil && (len(narrowed.Efforts) > 0 || narrowed.Budget != nil) {
		return fmt.Errorf("ModelServing %q declares reasoning modes on a Model with no reasoning definition", serving.Name)
	}
	for _, effort := range narrowed.Efforts {
		if !slices.Contains(canonical.Efforts, effort) {
			return fmt.Errorf("ModelServing %q reasoning effort %q is not part of Model %q's %v", serving.Name, effort, model.Name, canonical.Efforts)
		}
	}
	// A budget bound the provider does not publish is not a bound the serving
	// may narrow: a model that publishes only a ceiling says nothing about its
	// floor, and a serving that states one is describing its own range. A model
	// that publishes no budget at all supports no budget-based reasoning, so a
	// serving may not invent one.
	if narrowed.Budget != nil {
		if canonical.Budget == nil {
			return fmt.Errorf("ModelServing %q declares a reasoning budget on a Model %q with no budget range", serving.Name, model.Name)
		}
		if narrowed.Budget.Min != nil && canonical.Budget.Min != nil && *narrowed.Budget.Min < *canonical.Budget.Min {
			return fmt.Errorf("ModelServing %q reasoning budget minimum widens Model %q", serving.Name, model.Name)
		}
		if narrowed.Budget.Max != nil && (canonical.Budget.Max == nil || *narrowed.Budget.Max > *canonical.Budget.Max) {
			return fmt.Errorf("ModelServing %q reasoning budget maximum widens Model %q", serving.Name, model.Name)
		}
		if narrowed.Budget.Min != nil && canonical.Budget.Max != nil && *narrowed.Budget.Min > *canonical.Budget.Max {
			return fmt.Errorf("ModelServing %q reasoning budget minimum %d exceeds Model %q maximum %d", serving.Name, *narrowed.Budget.Min, model.Name, *canonical.Budget.Max)
		}
		if narrowed.Budget.Max != nil && canonical.Budget.Min != nil && *narrowed.Budget.Max < *canonical.Budget.Min {
			return fmt.Errorf("ModelServing %q reasoning budget maximum %d is below Model %q minimum %d", serving.Name, *narrowed.Budget.Max, model.Name, *canonical.Budget.Min)
		}
		if err := validateReasoningBudget("ModelServing", serving.Name, narrowed.Budget); err != nil {
			return err
		}
	}
	return nil
}

// mergeReasoning composes a serving's reasoning override with its Model's fact
// sheet. The override replaces the fact sheet's EFFORTS with an explicit
// narrower set (a serving that publishes a different list is stating the levels
// its provider accepts).
//
// The budget follows the narrowing rule in two distinct ways, and confusing them
// silently widens the catalog:
//
//   - An ABSENT override budget means the serving exposes no budget-based
//     reasoning at all. Removing a capability is a narrowing, so it stays absent;
//     inheriting the Model's range here would claim the serving still offers it.
//   - A DECLARED override budget keeps the Model's bounds for whatever it omits,
//     because "no floor published" cannot mean "no floor" when the Model
//     publishes one — that would widen the accepted range.
//
// The result is a deep copy: no pointer from the input catalogs is shared with
// the caller.
func mergeReasoning(canonical, override *v1alpha1.ModelReasoning) *v1alpha1.ModelReasoning {
	if override == nil {
		return cloneReasoning(canonical)
	}
	merged := cloneReasoning(override)
	if canonical == nil || canonical.Budget == nil || merged.Budget == nil {
		return merged
	}
	if merged.Budget.Min == nil && canonical.Budget.Min != nil {
		minimum := *canonical.Budget.Min
		merged.Budget.Min = &minimum
	}
	if merged.Budget.Max == nil && canonical.Budget.Max != nil {
		maximum := *canonical.Budget.Max
		merged.Budget.Max = &maximum
	}
	return merged
}

func cloneReasoning(in *v1alpha1.ModelReasoning) *v1alpha1.ModelReasoning {
	if in == nil {
		return nil
	}
	out := *in
	out.Efforts = slices.Clone(in.Efforts)
	if in.Budget != nil {
		budget := *in.Budget
		if in.Budget.Min != nil {
			min := *in.Budget.Min
			budget.Min = &min
		}
		if in.Budget.Max != nil {
			max := *in.Budget.Max
			budget.Max = &max
		}
		out.Budget = &budget
	}
	return &out
}
