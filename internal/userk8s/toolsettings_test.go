package userk8s

import (
	"context"
	"strings"
	"testing"
)

// toolSettingsCatalog builds a catalog whose Model carries tool settings and
// whose Serving narrows the capabilities those settings were authored against.
func toolSettingsCatalog(toolSettings, overrides string) []string {
	return []string{
		`apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`,
		`apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-model
spec:
  canonical_id: alpha/alpha-model
  display_name: Alpha Model
  context_window: 100000
  max_output_tokens: 10000
` + toolSettings,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: alpha-serving
spec:
  provider: alpha
  model: alpha-model
  provider_model_id: alpha-wire
` + overrides,
	}
}

// TestToolSettingsMustFitTheEffectiveServing proves a tool setting is validated
// against the serving it is projected through, not only the canonical Model: an
// override that fits the Model can still exceed a narrowed serving, which would
// launch a tool configured beyond what the provider offers.
func TestToolSettingsMustFitTheEffectiveServing(t *testing.T) {
	tests := []struct {
		name         string
		toolSettings string
		overrides    string
		wantErr      string
	}{
		{
			name:         "override above a narrowed context without a waive",
			toolSettings: "  toolSettings:\n    claude:\n      context_window: 80000\n",
			overrides:    "  overrides:\n    context_window: 50000\n",
			wantErr:      "narrows Model",
		},
		{
			name:         "the same override with a waive is accepted",
			toolSettings: "  toolSettings:\n    claude:\n      context_window: 80000\n      waive: the tool must see the canonical budget\n",
			overrides:    "  overrides:\n    context_window: 50000\n",
		},
		{
			name:         "override above a narrowed output budget without a waive",
			toolSettings: "  toolSettings:\n    claude:\n      max_output_tokens: 8000\n",
			overrides:    "  overrides:\n    max_output_tokens: 4000\n",
			wantErr:      "narrows Model",
		},
		{
			name:         "override within the narrowed serving needs no waive",
			toolSettings: "  toolSettings:\n    claude:\n      context_window: 40000\n",
			overrides:    "  overrides:\n    context_window: 50000\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loadDocuments(toBytes(toolSettingsCatalog(test.toolSettings, test.overrides))...)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("loadDocuments: %v, want the catalog accepted", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error: got %v, want it to contain %q", err, test.wantErr)
			}
		})
	}
}

// TestToolSettingsEffortRemovedByServingIsRefused covers the reasoning half
// directly, because it needs a Model that publishes the effort the serving then
// removes.
func TestToolSettingsEffortRemovedByServingIsRefused(t *testing.T) {
	documents := []string{
		`apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`,
		`apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-model
spec:
  canonical_id: alpha/alpha-model
  display_name: Alpha Model
  context_window: 100000
  max_output_tokens: 10000
  reasoning:
    supported: true
    efforts:
    - low
    - high
  toolSettings:
    claude:
      effort_level: high
`,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: alpha-serving
spec:
  provider: alpha
  model: alpha-model
  provider_model_id: alpha-wire
  overrides:
    reasoning:
      supported: true
      efforts:
      - low
`,
		`apiVersion: one-shot-man/v1alpha1
kind: Tool
metadata:
  name: claude
spec:
  display_name: Claude Code
  surfaces:
  - anthropic
  credential_channels:
  - gateway
  identifiers:
    provider_slug:
      pattern: ^[a-z0-9][a-z0-9-]*$
    model_slug:
      pattern: ^[A-Za-z0-9][A-Za-z0-9._:-]*$
  budget_profile: claude-margin
`,
	}
	_, err := loadDocuments(toBytes(documents)...)
	if err == nil || !strings.Contains(err.Error(), "removed reasoning effort") {
		t.Fatalf("error: got %v, want the removed effort refused", err)
	}

	// With a waive reason the compromise is explicit and accepted.
	documents[1] = strings.Replace(documents[1], "      effort_level: high", "      effort_level: high\n      waive: legacy configuration", 1)
	objects, err := loadDocuments(toBytes(documents)...)
	if err != nil {
		t.Fatalf("loadDocuments with a waive: %v", err)
	}
	backend := mustBackend(t, objects, &fakeRunner{})
	projection, err := backend.Project(context.Background(), "claude", "alpha", "alpha-serving")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if got := projection.Settings["effort_level"]; got != "high" {
		t.Fatalf("settings.effort_level: got %v, want the waived high", got)
	}
}
