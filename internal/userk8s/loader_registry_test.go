package userk8s

import (
	"strings"
	"testing"
)

func TestLoadResolvesRegistryNames(t *testing.T) {
	provider := func(name string) string {
		return `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: ` + name + `
spec:
  display_name: ` + name + `
`
	}
	model := func(registryName, metadataName, providerName string, equivalents ...string) string {
		document := `apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: ` + metadataName + `
`
		if registryName != "" {
			document = `apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  annotations:
    one-shot-man/registry-name: ` + registryName + `
  name: ` + metadataName + `
`
		}
		document += `spec:
  provider: ` + providerName + `
  context_window: 1000
  max_output_tokens: 10
`
		if len(equivalents) > 0 {
			document += "  equivalent_to:\n"
			for _, equivalent := range equivalents {
				parts := strings.SplitN(equivalent, "/", 2)
				document += "  - provider: " + parts[0] + "\n    name: " + parts[1] + "\n"
			}
		}
		return document
	}

	t.Run("a sanitized name is referenced by its registry name", func(t *testing.T) {
		objects := documents(t,
			provider("alpha"),
			model("", "alpha-model", "alpha", "alpha/alpha-model:dev"),
			model("alpha-model:dev", "alpha-model-dev", "alpha", "alpha/alpha-model"),
		)
		if len(objects.Models) != 2 {
			t.Fatalf("models: got %d, want 2", len(objects.Models))
		}
	})

	t.Run("a metadata name colliding with another registry name resolves to the registry name", func(t *testing.T) {
		objects := documents(t,
			provider("wafer"),
			provider("deepseek"),
			model("DeepSeek-V4-Pro", "deepseek-v4-pro", "wafer", "deepseek/deepseek-v4-pro"),
			model("deepseek-v4-pro", "deepseek-v4-pro-2", "deepseek", "wafer/DeepSeek-V4-Pro"),
		)
		if len(objects.Models) != 2 {
			t.Fatalf("models: got %d, want 2", len(objects.Models))
		}
		wafer, ok := objects.lookupModel("DeepSeek-V4-Pro")
		if !ok {
			t.Fatal("lookupModel(DeepSeek-V4-Pro): not found")
		}
		if wafer.Spec.Provider != "wafer" {
			t.Fatalf("lookupModel(DeepSeek-V4-Pro): provider %q, want wafer", wafer.Spec.Provider)
		}
		deepseek, ok := objects.lookupModel("deepseek-v4-pro")
		if !ok {
			t.Fatal("lookupModel(deepseek-v4-pro): not found")
		}
		if deepseek.Spec.Provider != "deepseek" {
			t.Fatalf("lookupModel(deepseek-v4-pro): provider %q, want deepseek", deepseek.Spec.Provider)
		}
	})

	t.Run("equivalent references that disagree on provider are rejected", func(t *testing.T) {
		_, err := LoadDocuments([]byte(provider("alpha")), []byte(provider("beta")), []byte(
			model("", "alpha-model", "alpha", "beta/alpha-model")), []byte(
			model("", "alpha-model-peer", "beta", "alpha/alpha-model")))
		if err == nil {
			t.Fatal("LoadDocuments: want an error for a provider disagreement")
		}
		if !strings.Contains(err.Error(), "declares provider") {
			t.Fatalf("error: got %v", err)
		}
	})

	t.Run("defaults are unique per provider across registry spellings", func(t *testing.T) {
		defaultModel := func(name string) string {
			return `apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: ` + name + `
spec:
  provider: alpha
  context_window: 1000
  max_output_tokens: 10
  default: true
`
		}
		_, err := LoadDocuments([]byte(provider("alpha")), []byte(defaultModel("alpha-one")), []byte(defaultModel("alpha-two")))
		if err == nil {
			t.Fatal("LoadDocuments: want an error for two defaults under one provider")
		}
		if !strings.Contains(err.Error(), "both declare default") {
			t.Fatalf("error: got %v", err)
		}
	})
}
