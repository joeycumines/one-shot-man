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
		effective.Reasoning = cloneReasoning(overrides.Reasoning)
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
	if narrowed.Budget != nil {
		if narrowed.Budget.Min != nil && (canonical.Budget == nil || canonical.Budget.Min == nil || *narrowed.Budget.Min < *canonical.Budget.Min) {
			return fmt.Errorf("ModelServing %q reasoning budget minimum widens Model %q", serving.Name, model.Name)
		}
		if narrowed.Budget.Max != nil && (canonical.Budget == nil || canonical.Budget.Max == nil || *narrowed.Budget.Max > *canonical.Budget.Max) {
			return fmt.Errorf("ModelServing %q reasoning budget maximum widens Model %q", serving.Name, model.Name)
		}
	}
	return nil
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
