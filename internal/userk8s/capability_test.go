package userk8s

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

func reasoningModel(spec *v1alpha1.ModelReasoning) *v1alpha1.Model {
	return &v1alpha1.Model{
		Spec: v1alpha1.ModelSpec{
			CanonicalID:     "alpha/alpha-model",
			DisplayName:     "Alpha Model",
			ContextWindow:   1000,
			MaxOutputTokens: 100,
			Reasoning:       spec,
		},
	}
}

func reasoningServing(overrides *v1alpha1.ModelServingOverrides) *v1alpha1.ModelServing {
	serving := &v1alpha1.ModelServing{
		Spec: v1alpha1.ModelServingSpec{
			Provider:        "alpha",
			Model:           "alpha-model",
			ProviderModelID: "alpha-wire",
			Overrides:       overrides,
		},
	}
	serving.Name = "alpha-serving"
	return serving
}

// TestReasoningBudgetNarrowing pins the budget rule: a serving may lower a
// published ceiling, may add a floor to a ceiling-only model (the provider
// published no floor, so there is nothing to widen), and may not lower the
// published floor or raise the ceiling.
func TestReasoningBudgetNarrowing(t *testing.T) {
	tests := []struct {
		name      string
		canonical *v1alpha1.ModelReasoningBudget
		override  *v1alpha1.ModelReasoningBudget
		wantErr   bool
	}{
		{
			name:      "lowering the ceiling narrows",
			canonical: &v1alpha1.ModelReasoningBudget{Min: new(int64(1024)), Max: new(int64(32000))},
			override:  &v1alpha1.ModelReasoningBudget{Max: new(int64(16000))},
		},
		{
			name:      "adding a floor to a ceiling-only model is allowed",
			canonical: &v1alpha1.ModelReasoningBudget{Max: new(int64(32000))},
			override:  &v1alpha1.ModelReasoningBudget{Min: new(int64(1024))},
		},
		{
			name:      "lowering the floor widens",
			canonical: &v1alpha1.ModelReasoningBudget{Min: new(int64(1024))},
			override:  &v1alpha1.ModelReasoningBudget{Min: new(int64(512))},
			wantErr:   true,
		},
		{
			name:      "raising the ceiling widens",
			canonical: &v1alpha1.ModelReasoningBudget{Max: new(int64(32000))},
			override:  &v1alpha1.ModelReasoningBudget{Max: new(int64(64000))},
			wantErr:   true,
		},
		{
			name:     "inventing a budget on a model with none",
			override: &v1alpha1.ModelReasoningBudget{Max: new(int64(1000))},
			wantErr:  true,
		},
		{
			name:      "inverted override bounds",
			canonical: &v1alpha1.ModelReasoningBudget{Max: new(int64(32000))},
			override:  &v1alpha1.ModelReasoningBudget{Min: new(int64(2000)), Max: new(int64(1000))},
			wantErr:   true,
		},
		{
			name:      "override ceiling below canonical floor inverts after merge",
			canonical: &v1alpha1.ModelReasoningBudget{Min: new(int64(2000)), Max: new(int64(32000))},
			override:  &v1alpha1.ModelReasoningBudget{Max: new(int64(1000))},
			wantErr:   true,
		},
		{
			name:      "override floor above canonical ceiling inverts after merge",
			canonical: &v1alpha1.ModelReasoningBudget{Min: new(int64(1024)), Max: new(int64(10000))},
			override:  &v1alpha1.ModelReasoningBudget{Min: new(int64(20000))},
			wantErr:   true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := reasoningModel(&v1alpha1.ModelReasoning{
				Supported: true,
				Budget:    test.canonical,
			})
			serving := reasoningServing(&v1alpha1.ModelServingOverrides{
				Reasoning: &v1alpha1.ModelReasoning{
					Supported: true,
					Budget:    test.override,
				},
			})
			_, err := effectiveModel(model, serving)
			if (err != nil) != test.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, test.wantErr)
			}
		})
	}
}

func TestReasoningBudgetOverrideErrorAttribution(t *testing.T) {
	model := reasoningModel(&v1alpha1.ModelReasoning{
		Supported: true,
		Budget:    &v1alpha1.ModelReasoningBudget{Min: new(int64(1024)), Max: new(int64(32000))},
	})
	serving := reasoningServing(&v1alpha1.ModelServingOverrides{
		Reasoning: &v1alpha1.ModelReasoning{
			Supported: true,
			Budget:    &v1alpha1.ModelReasoningBudget{Min: new(int64(2000)), Max: new(int64(1000))},
		},
	})
	_, err := effectiveModel(model, serving)
	if err == nil {
		t.Fatal("expected error for inverted override bounds, got nil")
	}
	want := "ModelServing \"alpha-serving\""
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error message %q should name the ModelServing %q", err.Error(), want)
	}
}

// TestEffectiveReasoningMergesBudgetBounds proves the merged effective reasoning
// keeps a canonical floor the serving did not mention. Replacing the budget
// wholesale would erase it and widen the range — the defect the merge fixes.
func TestEffectiveReasoningMergesBudgetBounds(t *testing.T) {
	model := reasoningModel(&v1alpha1.ModelReasoning{
		Supported: true,
		Toggle:    true,
		Efforts:   []string{"low", "high"},
		Budget:    &v1alpha1.ModelReasoningBudget{Min: new(int64(1024)), Max: new(int64(32000))},
	})
	serving := reasoningServing(&v1alpha1.ModelServingOverrides{
		Reasoning: &v1alpha1.ModelReasoning{
			Supported: true,
			Toggle:    true,
			Budget:    &v1alpha1.ModelReasoningBudget{Max: new(int64(16000))},
		},
	})

	effective, err := effectiveModel(model, serving)
	if err != nil {
		t.Fatalf("effectiveModel: %v", err)
	}
	budget := effective.Reasoning.Budget
	if budget == nil || budget.Min == nil || *budget.Min != 1024 {
		t.Fatalf("budget min: got %+v, want the inherited canonical 1024", budget)
	}
	if budget.Max == nil || *budget.Max != 16000 {
		t.Fatalf("budget max: got %+v, want the narrowed 16000", budget)
	}
}

// TestEffectiveReasoningReplacesEfforts proves an override still states its own
// effort set rather than inheriting the canonical list: four servings in the
// shipped fixture depend on narrowing efforts this way.
func TestEffectiveReasoningReplacesEfforts(t *testing.T) {
	model := reasoningModel(&v1alpha1.ModelReasoning{
		Supported: true,
		Efforts:   []string{"low", "medium", "high", "xhigh", "max"},
	})
	serving := reasoningServing(&v1alpha1.ModelServingOverrides{
		Reasoning: &v1alpha1.ModelReasoning{
			Supported: true,
			Efforts:   []string{"low", "high"},
		},
	})

	effective, err := effectiveModel(model, serving)
	if err != nil {
		t.Fatalf("effectiveModel: %v", err)
	}
	got := effective.Reasoning.Efforts
	if len(got) != 2 || got[0] != "low" || got[1] != "high" {
		t.Fatalf("efforts: got %v, want the override's explicit [low high]", got)
	}
}

// TestEffectiveReasoningDoesNotAliasCatalogState proves the merge copies, so a
// consumer mutating the effective reasoning cannot corrupt the Model or Serving
// it came from.
func TestEffectiveReasoningDoesNotAliasCatalogState(t *testing.T) {
	model := reasoningModel(&v1alpha1.ModelReasoning{
		Supported: true,
		Efforts:   []string{"low", "high"},
		Budget:    &v1alpha1.ModelReasoningBudget{Min: new(int64(1024)), Max: new(int64(32000))},
	})
	serving := reasoningServing(&v1alpha1.ModelServingOverrides{
		Reasoning: &v1alpha1.ModelReasoning{
			Supported: true,
			Efforts:   []string{"low"},
			// The override declares a budget so the inherited floor is part of
			// the merged result; the assertions below then prove the copy.
			Budget: &v1alpha1.ModelReasoningBudget{Max: new(int64(16000))},
		},
	})

	effective, err := effectiveModel(model, serving)
	if err != nil {
		t.Fatalf("effectiveModel: %v", err)
	}
	*effective.Reasoning.Budget.Min = 1
	effective.Reasoning.Efforts[0] = "ultra"
	if *model.Spec.Reasoning.Budget.Min != 1024 {
		t.Errorf("mutating the effective budget changed the Model: got %d", *model.Spec.Reasoning.Budget.Min)
	}
	if model.Spec.Reasoning.Efforts[0] != "low" {
		t.Errorf("mutating the effective efforts changed the Model: got %v", model.Spec.Reasoning.Efforts)
	}
	if serving.Spec.Overrides.Reasoning.Efforts[0] != "low" {
		t.Errorf("mutating the effective efforts changed the Serving: got %v", serving.Spec.Overrides.Reasoning.Efforts)
	}
	if *serving.Spec.Overrides.Reasoning.Budget.Max != 16000 {
		t.Errorf("mutating the effective budget changed the Serving: got %d", *serving.Spec.Overrides.Reasoning.Budget.Max)
	}
}

// TestReasoningOverrideOmittingBudgetNarrows pins the other half of the merge
// rule: an override that declares no budget at all states that this provider
// takes no budget-based reasoning, which is a narrowing and not an omission to
// fill in from the Model.
func TestReasoningOverrideOmittingBudgetNarrows(t *testing.T) {
	model := reasoningModel(&v1alpha1.ModelReasoning{
		Supported: true,
		Budget:    &v1alpha1.ModelReasoningBudget{Min: new(int64(1024)), Max: new(int64(32000))},
	})
	serving := reasoningServing(&v1alpha1.ModelServingOverrides{
		Reasoning: &v1alpha1.ModelReasoning{Supported: true},
	})

	effective, err := effectiveModel(model, serving)
	if err != nil {
		t.Fatalf("effectiveModel: %v", err)
	}
	if effective.Reasoning.Budget != nil {
		t.Fatalf("budget: got %+v, want none — the override did not declare one", effective.Reasoning.Budget)
	}
}
