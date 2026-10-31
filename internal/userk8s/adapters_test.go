package userk8s

import (
	"context"
	"testing"
)

// TestAdapterRequestedSelectionsProject pins the projections the tool adapters
// actually request today: the identifier each wrapper passes is either kept
// verbatim or derived into the spelling that tool's configuration uses, and the
// raw registry identifier is always preserved alongside it.
func TestAdapterRequestedSelectionsProject(t *testing.T) {
	backend, err := NewFilesBackend(FilesBackendOptions{Paths: []string{renderedPersonalProfile}})
	if err != nil {
		t.Fatalf("NewFilesBackend: %v", err)
	}
	ctx := context.Background()

	tests := []struct {
		name     string
		tool     string
		provider string
		model    string
		wantSlug string
		wantID   string
	}{
		{
			name:     "claude keeps the electronhub registry id verbatim",
			tool:     "claude",
			provider: "electronhub",
			model:    "glm-5.3:dev:electronhub",
			wantSlug: "glm-5.3:dev:electronhub",
			wantID:   "glm-5.3:dev:electronhub",
		},
		{
			name:     "opencode derives the key its configuration uses",
			tool:     "opencode",
			provider: "electronhub",
			model:    "glm-5.3:dev:electronhub",
			wantSlug: "glm_5_3_dev_electronhub",
			wantID:   "glm-5.3:dev:electronhub",
		},
		{
			name:     "crush keeps the electronhub registry id verbatim",
			tool:     "crush",
			provider: "electronhub",
			model:    "glm-5.3:dev:electronhub",
			wantSlug: "glm-5.3:dev:electronhub",
			wantID:   "glm-5.3:dev:electronhub",
		},
		// The former codex case "~openai/gpt-latest" was removed with the model
		// itself (2026-09-23): its wire id is not expressible in the shaper's
		// -model-table grammar, so the catalog no longer carries it. Codex's
		// acceptance of slash-bearing ids stays covered by the full-catalog
		// sweep in profile_test (codex x openrouter/owl-alpha) and by the
		// remaining cases here.
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection, err := backend.Project(ctx, test.tool, test.provider, test.model)
			if err != nil {
				t.Fatalf("Project(%s,%s,%s): %v", test.tool, test.provider, test.model, err)
			}
			if projection.ModelSlug != test.wantSlug {
				t.Errorf("model slug: got %q, want %q", projection.ModelSlug, test.wantSlug)
			}
			if projection.ModelID != test.wantID {
				t.Errorf("model id: got %q, want %q", projection.ModelID, test.wantID)
			}
			if projection.ProviderSlug != test.provider {
				t.Errorf("provider slug: got %q, want %q", projection.ProviderSlug, test.provider)
			}
			if projection.BudgetProfile == "" {
				t.Error("budget profile: empty")
			}
			if _, ok := projection.Settings["context_window"].(int64); !ok {
				t.Errorf("settings.context_window: got %T", projection.Settings["context_window"])
			}
		})
	}
}

// TestProjectDerivesProviderSlugFromTheToolGrammar exercises the same
// raw-versus-derived rule for the provider identifier, using a provider name
// no ASCII tool grammar accepts as-is.
func TestProjectDerivesProviderSlugFromTheToolGrammar(t *testing.T) {
	objects := documents(t,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha+beta
spec:
  display_name: Alpha Beta
`,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha
spec:
  provider: alpha+beta
  protocol: chat
`,
		`apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-model
spec:
  canonical_id: alpha/alpha-model
  display_name: Alpha Model
  context_window: 1000
  max_output_tokens: 10
`,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: alpha-model-alpha
spec:
  provider: alpha+beta
  model: alpha-model
  provider_model_id: alpha-model
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
      pattern: ^[A-Za-z0-9][A-Za-z0-9._-]*$
    model_slug:
      pattern: ^[A-Za-z0-9][A-Za-z0-9._:\[\]-]*$
  budget_profile: claude-margin
`)
	backend := mustBackend(t, objects, &fakeRunner{})
	projection, err := backend.Project(context.Background(), "claude", "alpha+beta", "alpha-model-alpha")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if projection.ProviderSlug != "alpha_beta" {
		t.Fatalf("provider slug: got %q, want the derived alpha_beta", projection.ProviderSlug)
	}
	if projection.ProviderID != "alpha+beta" {
		t.Fatalf("provider id: got %q, want the raw registry spelling alpha+beta", projection.ProviderID)
	}
}
