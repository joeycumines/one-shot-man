package userk8smod

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/joeycumines/goja"
)

// renderedProfile is the engine package's frozen copy of a real rendered
// personal profile; the module tests load it through the scripting surface
// rather than restating catalog facts.
const renderedProfile = "../../userk8s/testdata/rendered-personal-profile.yaml"

type failingRunner struct{}

func (failingRunner) Run(context.Context, []string, time.Duration) (string, error) {
	return "", errors.New("test runner never resolves")
}

func newRuntime(t *testing.T, options Options) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	module := vm.NewObject()
	exports := vm.NewObject()
	if err := module.Set("exports", exports); err != nil {
		t.Fatalf("setting module exports: %v", err)
	}
	Require(context.Background(), options)(vm, module)
	if err := vm.Set("userk8s", exports); err != nil {
		t.Fatalf("exposing the module: %v", err)
	}
	return vm
}

func TestModuleLoadsCatalogFromArtifacts(t *testing.T) {
	vm := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}})
	value, err := vm.RunString(`
		const loaded = userk8s.load();
		({
			source: loaded.source,
			countsOk: loaded.providers.length === 13 && loaded.models.length === 158
				&& loaded.accesses.length === 15 && loaded.tools.length === 9 && loaded.secretsPresent === 15,
			namesOk: loaded.providers[0].name.length > 0 && loaded.providers[0].displayName.length > 0
				&& loaded.tools[0].name.length > 0 && loaded.tools[0].budgetProfile.length > 0,
			schemesOk: loaded.accesses.every(function (access) {
				return typeof access.scheme === "string" && access.scheme.length > 0 && access.requiredEnv.length > 0;
			}),
			registryOk: loaded.models.some(function (model) { return model.registryName === "glm-5.3:dev"; }),
			noSecretMaterial: loaded.accesses.every(function (access) {
				return access.value === undefined && access.credentials === undefined;
			})
		})
	`)
	if err != nil {
		t.Fatalf("load(): %v", err)
	}
	got := value.Export().(map[string]any)
	for _, want := range []struct {
		key  string
		want any
	}{
		{"source", "files"},
		{"countsOk", true},
		{"namesOk", true},
		{"schemesOk", true},
		{"registryOk", true},
		{"noSecretMaterial", true},
	} {
		if got[want.key] != want.want {
			t.Errorf("load().%s: got %v, want %v", want.key, got[want.key], want.want)
		}
	}
}

func TestModuleProjectsSelection(t *testing.T) {
	vm := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}})
	value, err := vm.RunString(`
		const projection = userk8s.project("claude", "electronhub", "glm-5.3:dev");
		({
			providerSlug: projection.providerSlug,
			modelSlug: projection.modelSlug,
			budgetProfile: projection.budgetProfile,
			access: projection.access,
			surfacesOk: projection.surfaces.length > 0,
			numbersOk: projection.settings.context_window === 262000
				&& projection.settings.max_output_tokens === 16384
				&& projection.settings.can_reason === true
		})
	`)
	if err != nil {
		t.Fatalf("project(): %v", err)
	}
	got := value.Export().(map[string]any)
	for _, want := range []struct {
		key  string
		want any
	}{
		{"providerSlug", "electronhub"},
		{"modelSlug", "glm-5.3:dev"},
		{"budgetProfile", "claude-margin"},
		{"access", "electronhub-shaper"},
		{"surfacesOk", true},
		{"numbersOk", true},
	} {
		if got[want.key] != want.want {
			t.Errorf("project().%s: got %v, want %v", want.key, got[want.key], want.want)
		}
	}
}

func TestModuleResolveCredentialReportsStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	vm := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}, Runner: failingRunner{}})
	value, err := vm.RunString(`
		const resolution = userk8s.resolveCredential("electronhub-shaper");
		({
			status: resolution.status,
			hasReason: typeof resolution.reason === "string" && resolution.reason.length > 0,
			noCredentials: (resolution.credentials || []).length === 0,
			namesSlot: resolution.reason.indexOf("ELECTRONHUB_API_KEY") >= 0 || resolution.reason.indexOf("API_KEY") >= 0
		})
	`)
	if err != nil {
		t.Fatalf("resolveCredential(): %v", err)
	}
	got := value.Export().(map[string]any)
	if got["status"] != "missingCredentials" {
		t.Errorf("resolveCredential().status: got %v, want missingCredentials", got["status"])
	}
	if got["hasReason"] != true || got["noCredentials"] != true {
		t.Errorf("resolveCredential(): got %v", got)
	}
}

func TestModuleBackendStatus(t *testing.T) {
	vm := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}})
	value, err := vm.RunString(`
		const status = userk8s.backendStatus();
		({
			source: status.source,
			ok: status.providers === 13 && status.models === 158 && status.bindings === 15 && status.artifacts.length === 1
		})
	`)
	if err != nil {
		t.Fatalf("backendStatus(): %v", err)
	}
	got := value.Export().(map[string]any)
	if got["source"] != "files" || got["ok"] != true {
		t.Errorf("backendStatus(): got %v", got)
	}
}

func TestModuleResolveCredentialRequiresReference(t *testing.T) {
	vm := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}})
	if _, err := vm.RunString(`userk8s.resolveCredential("")`); err == nil {
		t.Fatal("resolveCredential(\"\"): want an error")
	}
	if _, err := vm.RunString(`userk8s.project("claude", "electronhub", "")`); err == nil {
		t.Fatal("project with an empty model: want an error")
	}
	if _, err := vm.RunString(`userk8s.resolveCredential("no-such-access")`); err == nil {
		t.Fatal("resolveCredential(unknown): want an error")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestModuleProjectsReconciledSlug(t *testing.T) {
	vm := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}})
	value, err := vm.RunString(`
		const projection = userk8s.project("opencode", "electronhub", "glm-5.3:dev");
		({
			providerSlug: projection.providerSlug,
			modelSlug: projection.modelSlug,
			modelId: projection.modelId,
			budgetProfile: projection.budgetProfile,
			settingsOk: projection.settings.context_window === 262000 && projection.settings.max_output_tokens === 16384
		})
	`)
	if err != nil {
		t.Fatalf("project(): %v", err)
	}
	got := value.Export().(map[string]any)
	for _, want := range []struct {
		key  string
		want any
	}{
		{"providerSlug", "electronhub"},
		{"modelSlug", "glm_5_3_dev"},
		{"modelId", "glm-5.3:dev"},
		{"budgetProfile", "sdk-limits"},
		{"settingsOk", true},
	} {
		if got[want.key] != want.want {
			t.Errorf("project().%s: got %v, want %v", want.key, got[want.key], want.want)
		}
	}
}

type resolvingRunner struct{}

func (resolvingRunner) Run(context.Context, []string, time.Duration) (string, error) {
	return "resolved-bearer-value", nil
}

func TestModuleResolvesBearerCredentialThroughJS(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	vm := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}, Runner: resolvingRunner{}})
	value, err := vm.RunString(`
		const resolution = userk8s.resolveCredential("electronhub-shaper");
		const credential = (resolution.credentials || [])[0];
		({
			status: resolution.status,
			envVar: credential && credential.envVar,
			valuePresent: !!credential && credential.value.length > 0,
			provenanceOk: !!credential && credential.provenance.indexOf("command:") === 0
		})
	`)
	if err != nil {
		t.Fatalf("resolveCredential(): %v", err)
	}
	got := value.Export().(map[string]any)
	if got["status"] != "resolved" {
		t.Fatalf("resolveCredential().status: got %v, want resolved", got["status"])
	}
	if got["envVar"] != "ELECTRONHUB_API_KEY" {
		t.Errorf("resolveCredential().credentials[0].envVar: got %v, want ELECTRONHUB_API_KEY", got["envVar"])
	}
	if got["valuePresent"] != true || got["provenanceOk"] != true {
		t.Errorf("resolveCredential(): got %v", got)
	}
}

func TestModuleRejectsUnserviceableSources(t *testing.T) {
	for _, test := range []struct {
		name    string
		options Options
	}{
		{name: "unknown source", options: Options{Source: "bogus", Artifacts: []string{renderedProfile}}},
		{name: "cluster without a backend", options: Options{Source: SourceCluster, Artifacts: []string{renderedProfile}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			vm := newRuntime(t, test.options)
			if _, err := vm.RunString(`userk8s.load()`); err == nil {
				t.Fatalf("load(): want an error for %s", test.name)
			}
		})
	}
}
