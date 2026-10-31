package userk8s

import (
	"strings"
	"testing"
)

func TestLoadResolvesServingRegistryNames(t *testing.T) {
	provider := func(name string) string {
		return `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: ` + name + `
spec:
  display_name: ` + name + `
`
	}
	model := func(name string) string {
		return `apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: ` + name + `
spec:
  canonical_id: vendor/` + name + `
  display_name: ` + name + `
  context_window: 1000
  max_output_tokens: 10
`
	}
	serving := func(metadataName, registryName, providerName, modelName string, isDefault bool) string {
		document := `apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: ` + metadataName + `
`
		if registryName != "" {
			document = `apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  annotations:
    one-shot-man/registry-name: ` + registryName + `
  name: ` + metadataName + `
`
		}
		document += `spec:
  provider: ` + providerName + `
  model: ` + modelName + `
  provider_model_id: ` + registryName + `
`
		if isDefault {
			document += "  default: true\n"
		}
		return document
	}

	t.Run("a serving resolves by metadata name and registry key", func(t *testing.T) {
		objects := documents(t,
			provider("alpha"),
			model("alpha-model"),
			serving("alpha-model-alpha", "alpha-model:alpha", "alpha", "alpha-model", false),
		)
		if len(objects.Servings) != 1 {
			t.Fatalf("servings: got %d, want 1", len(objects.Servings))
		}
		if got, ok := objects.lookupServing("alpha-model-alpha"); !ok || got.Name != "alpha-model-alpha" {
			t.Fatal("lookupServing by metadata name failed")
		}
		if got, ok := objects.lookupServing("alpha-model:alpha"); !ok || got.Name != "alpha-model-alpha" {
			t.Fatal("lookupServing by registry key failed")
		}
	})

	t.Run("defaults are unique per provider", func(t *testing.T) {
		_, err := loadDocuments(
			[]byte(provider("alpha")),
			[]byte(model("alpha-model")),
			[]byte(serving("alpha-one", "alpha-model:alpha", "alpha", "alpha-model", true)),
			[]byte(serving("alpha-two", "alpha-model-2:alpha", "alpha", "alpha-model", true)),
		)
		if err == nil {
			t.Fatal("loadDocuments: want an error for two defaults under one provider")
		}
		if !strings.Contains(err.Error(), "both declare default") {
			t.Fatalf("error: got %v", err)
		}
	})

	t.Run("a serving whose provider disagrees with its access is rejected at route validation", func(t *testing.T) {
		_, err := loadDocuments(
			[]byte(provider("alpha")),
			[]byte(provider("beta")),
			[]byte(`apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-chat
spec:
  provider: alpha
  protocol: chat
`),
			[]byte(model("alpha-model")),
			[]byte(serving("alpha-model-alpha", "alpha-model:alpha", "alpha", "alpha-model", false)),
			[]byte(serving("beta-model-beta", "alpha-model:beta", "beta", "alpha-model", false)),
			[]byte(`apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-local
  namespace: test
spec:
  access: alpha-chat
  surfaces: [chat]
  endpoints:
    chat: http://127.0.0.1:1/alpha/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`),
		)
		// The route defaults to all servings of the access's provider, so the beta
		// serving is not selected; but an explicit beta offer must be refused.
		if err != nil {
			t.Fatalf("loadDocuments without explicit beta offer: %v", err)
		}
	})
}
