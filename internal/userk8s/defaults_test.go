package userk8s

import (
	"context"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

// TestFilesDefaultsMatchClusterDefaults pins the CRD defaults the files backend
// must apply itself. The API server defaults an object before any backend sees
// it; a file read from disk has no API server, so an omitted field must still
// resolve to the same value the cluster backend would report. Each case writes
// the field nowhere and asserts the defaulting value, plus the explicit-zero
// counterpart where the zero value is meaningful.
func TestFilesDefaultsMatchClusterDefaults(t *testing.T) {
	model := `apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-model
spec:
  canonical_id: alpha/alpha-model
  display_name: Alpha Model
  context_window: 1000
  max_output_tokens: 10
`
	provider := `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`
	serving := `apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: alpha-serving
spec:
  provider: alpha
  model: alpha-model
  provider_model_id: alpha-wire
`
	binding := `apiVersion: one-shot-man/v1alpha1
kind: LocalSecretBinding
metadata:
  name: bind-alpha
  namespace: test
spec:
  selector:
    matchLabels:
      one-shot-man/provider: alpha
  secretRef:
    name: creds
    key: ALPHA_API_KEY
  envVar: ALPHA_API_KEY
  resolvers:
    - env:
        name: ALPHA_TOKEN
`

	t.Run("model tool_call and modalities default", func(t *testing.T) {
		objects := documents(t, provider, model, serving)
		got := objects.Models[0].Spec
		if !got.ToolCall {
			t.Error("tool_call: an omitted tool_call must default to true, as the API server would")
		}
		if len(got.InputModalities) != 1 || got.InputModalities[0] != "text" {
			t.Errorf("input_modalities: got %v, want [text]", got.InputModalities)
		}
		if len(got.OutputModalities) != 1 || got.OutputModalities[0] != "text" {
			t.Errorf("output_modalities: got %v, want [text]", got.OutputModalities)
		}
	})

	t.Run("explicit false is not overwritten by the default", func(t *testing.T) {
		objects := documents(t, provider, `apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-model
spec:
  canonical_id: alpha/alpha-model
  display_name: Alpha Model
  context_window: 1000
  max_output_tokens: 10
  tool_call: false
  input_modalities: []
`, serving)
		got := objects.Models[0].Spec
		if got.ToolCall {
			t.Error("tool_call: an explicit false must survive defaulting")
		}
		if got.InputModalities == nil || len(got.InputModalities) != 0 {
			t.Errorf("input_modalities: an explicit empty list must survive defaulting, got %v", got.InputModalities)
		}
	})

	t.Run("binding scheme defaults to bearer and resolves credentials", func(t *testing.T) {
		t.Setenv("ALPHA_TOKEN", "alpha-value")
		objects := documents(t, provider, model, serving, binding)
		if got := objects.Bindings[0].Spec.EffectiveScheme(); got != v1alpha1.CredentialBearer {
			t.Fatalf("EffectiveScheme: got %q, want the bearer default", got)
		}
		resolution, err := mustBackend(t, objects, &fakeRunner{}).ResolveCredential(context.Background(), "bind-alpha")
		if err != nil {
			t.Fatalf("ResolveCredential: %v", err)
		}
		if resolution.Status != StatusResolved || len(resolution.Credentials) != 1 {
			t.Fatalf("resolution: got %+v, want one resolved credential (an omitted scheme must not mean credential-less)", resolution)
		}
		if resolution.Credentials[0].Value != "alpha-value" {
			t.Fatalf("credential value: got %q", resolution.Credentials[0].Value)
		}
	})

	t.Run("an explicit none scheme stays credential-less", func(t *testing.T) {
		objects := documents(t, provider, model, serving, strings.Replace(binding, "  envVar: ALPHA_API_KEY", "  scheme: none\n  envVar: ALPHA_API_KEY", 1))
		resolution, err := mustBackend(t, objects, &fakeRunner{}).ResolveCredential(context.Background(), "bind-alpha")
		if err != nil {
			t.Fatalf("ResolveCredential: %v", err)
		}
		if resolution.Status != StatusResolved || len(resolution.Credentials) != 0 {
			t.Fatalf("resolution: got %+v, want resolved with no credentials", resolution)
		}
	})

	t.Run("serving priority and route order default to 100", func(t *testing.T) {
		objects := documents(t, provider, model, serving, `apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-chat
spec:
  provider: alpha
  protocol: chat
`,
			`apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-local
  namespace: test
spec:
  access: alpha-chat
  offers: []
  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:1/alpha/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`)
		if got := objects.Servings[0].Spec.Priority; got != 100 {
			t.Errorf("priority: got %d, want the default 100", got)
		}
		if got := objects.Routes[0].Spec.Order; got != 100 {
			t.Errorf("order: got %d, want the default 100", got)
		}
		if got := objects.Servings[0].Spec.EffectivePriority(); got != 100 {
			t.Errorf("EffectivePriority: got %d, want 100", got)
		}
		if got := objects.Routes[0].Spec.EffectiveOrder(); got != 100 {
			t.Errorf("EffectiveOrder: got %d, want 100", got)
		}
	})
}
