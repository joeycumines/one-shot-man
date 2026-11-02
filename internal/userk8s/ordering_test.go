package userk8s

import (
	"strings"
	"testing"
)

// TestExplicitZeroOrderAndPriorityAreRejected pins the normalization rule: 0 is
// the unset zero value, so an omitted field takes the CRD default of 100. An
// explicitly written 0 cannot be distinguished from "unset" after decoding, so it
// is rejected rather than silently rewritten — the author meant something by it,
// and 0 is not a meaningful preference. The CRDs state the same with Minimum=1.
func TestExplicitZeroOrderAndPriorityAreRejected(t *testing.T) {
	provider := `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`
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
	access := `apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-chat
spec:
  provider: alpha
  protocol: chat
`
	serving := func(extra string) string {
		return `apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: alpha-serving
spec:
  provider: alpha
  model: alpha-model
  provider_model_id: alpha-wire
` + extra
	}
	route := func(extra string) string {
		return `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-local
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-serving
  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:11239/alpha/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
` + extra
	}

	t.Run("explicit priority 0", func(t *testing.T) {
		_, err := loadDocuments(toBytes([]string{provider, model, serving("  priority: 0\n")})...)
		if err == nil || !strings.Contains(err.Error(), "declares priority 0") {
			t.Fatalf("error: got %v, want priority 0 refused", err)
		}
	})
	t.Run("explicit negative priority", func(t *testing.T) {
		_, err := loadDocuments(toBytes([]string{provider, model, serving("  priority: -1\n")})...)
		if err == nil || !strings.Contains(err.Error(), "must be at least 1") {
			t.Fatalf("error: got %v, want negative priority refused", err)
		}
	})
	t.Run("explicit order 0", func(t *testing.T) {
		_, err := loadDocuments(toBytes([]string{provider, model, access, serving(""), route("  order: 0\n")})...)
		if err == nil || !strings.Contains(err.Error(), "declares order 0") {
			t.Fatalf("error: got %v, want order 0 refused", err)
		}
	})
	t.Run("explicit negative order", func(t *testing.T) {
		_, err := loadDocuments(toBytes([]string{provider, model, access, serving(""), route("  order: -1\n")})...)
		if err == nil || !strings.Contains(err.Error(), "must be at least 1") {
			t.Fatalf("error: got %v, want negative order refused", err)
		}
	})
	t.Run("omitted order and priority take the default 100", func(t *testing.T) {
		objects := documents(t, provider, model, access, serving(""), route(""))
		if got := objects.Servings[0].Spec.Priority; got != 100 {
			t.Errorf("priority: got %d, want the default 100", got)
		}
		if got := objects.Routes[0].Spec.Order; got != 100 {
			t.Errorf("order: got %d, want the default 100", got)
		}
	})
	t.Run("an explicit non-zero order and priority are kept", func(t *testing.T) {
		objects := documents(t, provider, model, access, serving("  priority: 10\n"), route("  order: 20\n"))
		if got := objects.Servings[0].Spec.Priority; got != 10 {
			t.Errorf("priority: got %d, want 10", got)
		}
		if got := objects.Routes[0].Spec.Order; got != 20 {
			t.Errorf("order: got %d, want 20", got)
		}
		if got := objects.Routes[0].Spec.EffectiveOrder(); got != 20 {
			t.Errorf("EffectiveOrder: got %d, want 20", got)
		}
	})
}
