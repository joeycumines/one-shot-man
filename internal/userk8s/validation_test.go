package userk8s

import (
	"strings"
	"testing"
)

// TestFilesValidationMirrorsCELRules proves the files backend enforces the
// invariants the CRDs state in CEL. Each case is a document a cluster would
// reject; without these checks it would load from a rendered artifact, so the
// same catalog would behave differently depending on its backend.
func TestFilesValidationMirrorsCELRules(t *testing.T) {
	provider := `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`
	access := `apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-chat
spec:
  provider: alpha
  protocol: chat
`
	model := func(spec string) string {
		return `apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-model
spec:
  canonical_id: alpha/alpha-model
  display_name: Alpha Model
  context_window: 1000
  max_output_tokens: 10
` + spec
	}
	serving := func(provider, name string) string {
		return `apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: ` + name + `
spec:
  provider: ` + provider + `
  model: alpha-model
  provider_model_id: ` + name + `
`
	}
	route := func(transport string, offers string) string {
		if offers == "" {
			offers = "  offers:\n  - serving: alpha-serving\n"
		}
		return `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-local
  namespace: test
spec:
  access: alpha-chat
` + offers + `  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:11239/alpha/v1
  naming: served
` + transport
	}
	shaperTransport := "  transport:\n    kind: shaper\n    upstream: https://alpha.example\n"
	servedOffer := "  offers:\n  - serving: alpha-serving\n"

	tests := []struct {
		name    string
		docs    []string
		wantErr string
	}{
		{
			name:    "model without canonical_id",
			docs:    []string{provider, strings.Replace(model(""), "  canonical_id: alpha/alpha-model\n", "", 1)},
			wantErr: "declares no canonical_id",
		},
		{
			name:    "model without display_name",
			docs:    []string{provider, strings.Replace(model(""), "  display_name: Alpha Model\n", "", 1)},
			wantErr: "declares no display_name",
		},
		{
			name:    "model with non-positive context_window",
			docs:    []string{provider, strings.Replace(model(""), "  context_window: 1000", "  context_window: 0", 1)},
			wantErr: "context_window 0",
		},
		{
			name:    "model with unknown modality",
			docs:    []string{provider, model("  input_modalities:\n  - hologram\n")},
			wantErr: "input modality \"hologram\"",
		},
		{
			name:    "model with duplicate modality",
			docs:    []string{provider, model("  input_modalities:\n  - text\n  - text\n")},
			wantErr: "input modality \"text\" twice",
		},
		{
			name:    "model reasoning mode without supported",
			docs:    []string{provider, model("  reasoning:\n    efforts:\n    - low\n")},
			wantErr: "does not set supported: true",
		},
		{
			name:    "model with unknown reasoning effort",
			docs:    []string{provider, model("  reasoning:\n    supported: true\n    efforts:\n    - turbo\n")},
			wantErr: "reasoning effort \"turbo\"",
		},
		{
			name:    "model with inverted reasoning budget",
			docs:    []string{provider, model("  reasoning:\n    supported: true\n    budget:\n      min: 100\n      max: 10\n")},
			wantErr: "above its max",
		},
		{
			name:    "model with non-positive reasoning max",
			docs:    []string{provider, model("  reasoning:\n    supported: true\n    budget:\n      max: 0\n")},
			wantErr: "must be positive",
		},
		{
			name: "tool setting exceeding a fact without a waive",
			docs: []string{provider, model("  toolSettings:\n    claude:\n      context_window: 2000\n")},
			// The Model's context_window is 1000, so 2000 needs a waive.
			wantErr: "without a waive reason",
		},
		{
			name: "tool setting exceeding a fact with a waive",
			docs: []string{provider, model("  toolSettings:\n    claude:\n      context_window: 2000\n      waive: provider raises the cap\n")},
			// Accepted: no expected error. See wantErr == "".
		},
		{
			name:    "tool setting effort outside the model's efforts",
			docs:    []string{provider, model("  reasoning:\n    supported: true\n    efforts:\n    - low\n  toolSettings:\n    claude:\n      effort_level: high\n")},
			wantErr: "is not one of the model's reasoning efforts",
		},
		{
			name:    "access with unknown protocol",
			docs:    []string{provider, strings.Replace(access, "protocol: chat", "protocol: carrier-pigeon", 1)},
			wantErr: "unknown protocol \"carrier-pigeon\"",
		},
		{
			name: "selection with a candidate setting two references",
			docs: []string{provider, model(""), serving("alpha", "alpha-serving"),
				`apiVersion: one-shot-man/v1alpha1
kind: ModelSelection
metadata:
  name: alpha-default
spec:
  display_name: Alpha default
  candidates:
  - serving: alpha-serving
    model: alpha-model
`},
			wantErr: "sets 2 references",
		},
		{
			name: "selection with a candidate setting no reference",
			docs: []string{provider,
				`apiVersion: one-shot-man/v1alpha1
kind: ModelSelection
metadata:
  name: alpha-default
spec:
  display_name: Alpha default
  candidates:
  - {}
`},
			wantErr: "sets 0 references",
		},
		{
			name:    "selection without display_name",
			docs:    []string{provider, strings.Replace(selectionDocument(), "  display_name: Alpha default\n", "", 1)},
			wantErr: "declares no display_name",
		},
		{
			name:    "selection with unknown strategy",
			docs:    []string{provider, strings.Replace(selectionDocument(), "  strategy: ordered", "  strategy: random", 1)},
			wantErr: "unknown strategy \"random\"",
		},
		{
			name:    "route offer with a malformed id",
			docs:    []string{provider, model(""), serving("alpha", "alpha-serving"), route(shaperTransport, "  offers:\n  - serving: alpha-serving\n    id: Not_An_ID\n")},
			wantErr: "must be an RFC-1123 name",
		},
		{
			name:    "shaper route with a relative upstream",
			docs:    []string{provider, model(""), serving("alpha", "alpha-serving"), route("  transport:\n    kind: shaper\n    upstream: alpha.example\n", servedOffer)},
			wantErr: "is not an absolute URL",
		},
		{
			name:    "shaper route with an unknown auth mode",
			docs:    []string{provider, model(""), serving("alpha", "alpha-serving"), route("  transport:\n    kind: shaper\n    upstream: https://alpha.example\n    shaper:\n      auth_mode: monkey\n", servedOffer)},
			wantErr: "unknown shaper auth_mode",
		},
		{
			name: "shaper route accepted when everything agrees",
			docs: []string{provider, access, model(""), serving("alpha", "alpha-serving"), route(shaperTransport, servedOffer)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loadDocuments(toBytes(test.docs)...)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("loadDocuments: %v, want the document accepted", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("loadDocuments: want an error containing %q", test.wantErr)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error: got %q, want it to contain %q", err, test.wantErr)
			}
		})
	}
}

// selectionDocument is a minimal valid ModelSelection over a serving reference.
func selectionDocument() string {
	return `apiVersion: one-shot-man/v1alpha1
kind: ModelSelection
metadata:
  name: alpha-default
spec:
  display_name: Alpha default
  strategy: ordered
  candidates:
  - serving: alpha-serving
`
}
