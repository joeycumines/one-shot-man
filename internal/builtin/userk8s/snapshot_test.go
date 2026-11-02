package userk8smod

import (
	"os"
	"path/filepath"
	"testing"
)

// writeCatalog writes one multi-document catalog into a temp dir and returns its
// path. Tests that need a shape the shipped fixture does not carry use this
// rather than a second committed fixture.
func writeCatalog(t *testing.T, document string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "catalog.yaml")
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatalf("writing catalog: %v", err)
	}
	return path
}

// TestModuleResolvesServingModelByRegistryName proves the JS snapshot resolves a
// serving's Model reference in registry vocabulary. The engine accepts either
// spelling, so a metadata-name-only index would silently lose the model's
// displayName and its deprecated flag.
func TestModuleResolvesServingModelByRegistryName(t *testing.T) {
	catalog := `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
---
apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-model
  annotations:
    one-shot-man/registry-name: alpha:model
spec:
  canonical_id: alpha/alpha-model
  display_name: Alpha Model (human)
  context_window: 1000
  max_output_tokens: 10
  deprecated: true
---
apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: alpha-serving
spec:
  provider: alpha
  model: alpha:model
  provider_model_id: alpha-wire
`
	_, runAsync := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{writeCatalog(t, catalog)}})
	value, err := runAsync(`
		const loaded = await userk8s.load();
		const serving = loaded.servings[0];
		__collect({ displayName: serving.displayName, deprecated: serving.deprecated === true })
	`)
	if err != nil {
		t.Fatalf("load(): %v", err)
	}
	got := value.Export().(map[string]any)
	if got["displayName"] != "Alpha Model (human)" {
		t.Errorf("serving.displayName: got %v, want the model's display name (a registry-named Model reference must resolve)", got["displayName"])
	}
	if got["deprecated"] != true {
		t.Errorf("serving.deprecated: got %v, want true (the deprecated flag comes from the referenced Model)", got["deprecated"])
	}
}

// TestModuleExposesCamelCaseOverrides proves serving.overrides uses the same
// camelCase vocabulary as serving.effective. The snake_case CRD tags would force
// every script to know both spellings.
func TestModuleExposesCamelCaseOverrides(t *testing.T) {
	catalog := `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
---
apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-model
spec:
  canonical_id: alpha/alpha-model
  display_name: Alpha Model
  context_window: 100000
  max_output_tokens: 10000
---
apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: alpha-serving
spec:
  provider: alpha
  model: alpha-model
  provider_model_id: alpha-wire
  overrides:
    context_window: 50000
    max_output_tokens: 5000
    tool_call: false
    structured_output: false
`
	_, runAsync := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{writeCatalog(t, catalog)}})
	value, err := runAsync(`
		const loaded = await userk8s.load();
		const serving = loaded.servings[0];
		const keys = Object.keys(serving.overrides || {}).sort();
		__collect({
			keys: keys,
			contextWindow: serving.overrides.contextWindow,
			snakeCaseAbsent: serving.overrides.context_window === undefined,
			effectiveMatches: serving.overrides.contextWindow === 50000
				&& serving.effective.contextWindow === 50000
		})
	`)
	if err != nil {
		t.Fatalf("load(): %v", err)
	}
	got := value.Export().(map[string]any)
	if got["contextWindow"] != int64(50000) {
		t.Errorf("overrides.contextWindow: got %v, want 50000", got["contextWindow"])
	}
	if got["snakeCaseAbsent"] != true {
		t.Errorf("overrides must not carry the snake_case CRD spelling; got keys %v", got["keys"])
	}
	if got["effectiveMatches"] != true {
		t.Errorf("overrides and effective must use one vocabulary: %v", got)
	}
}

// TestModuleBindingSchemeIsDefaulted proves the JS binding view reports the
// scheme the cluster would: an omitted scheme is bearer, never an empty string.
func TestModuleBindingSchemeIsDefaulted(t *testing.T) {
	catalog := `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
---
apiVersion: one-shot-man/v1alpha1
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
---
apiVersion: one-shot-man/v1alpha1
kind: LocalSecretBinding
metadata:
  name: bind-none
  namespace: test
spec:
  selector:
    matchLabels:
      one-shot-man/provider: alpha
  secretRef:
    name: creds
    key: NONE
  scheme: none
  envVar: UNUSED
`
	_, runAsync := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{writeCatalog(t, catalog)}})
	value, err := runAsync(`
		const loaded = await userk8s.load();
		const schemes = {};
		loaded.bindings.forEach(function (binding) { schemes[binding.name] = binding.scheme; });
		__collect(schemes)
	`)
	if err != nil {
		t.Fatalf("load(): %v", err)
	}
	got := value.Export().(map[string]any)
	if got["bind-alpha"] != "bearer" {
		t.Errorf("bind-alpha.scheme: got %v, want the bearer default", got["bind-alpha"])
	}
	if got["bind-none"] != "none" {
		t.Errorf("bind-none.scheme: got %v, want the explicit none", got["bind-none"])
	}
}
