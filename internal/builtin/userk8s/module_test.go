package userk8smod

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// renderedProfile is the engine package's frozen copy of a real rendered
// personal profile; the module tests load it through the scripting surface
// rather than restating catalog facts.
const renderedProfile = "../../userk8s/testdata/rendered-personal-profile.yaml"

type failingRunner struct{}

func (failingRunner) Run(context.Context, []string, time.Duration) (string, error) {
	return "", errors.New("test runner never resolves")
}

// newRuntime builds the module against a live event loop and returns the
// runtime plus a runAsync helper that executes an async script body and
// collects either the awaited value (delivered to __collect) or the first
// rejection (__collectErr). The loop runs until the helper's deadline.
func newRuntime(t *testing.T, options Options) (*goja.Runtime, func(string) (goja.Value, error)) {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("event loop: %v", err)
	}
	vm := goja.New()
	adapter, err := gojaeventloop.New(loop, vm)
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("adapter bind: %v", err)
	}
	module := vm.NewObject()
	exports := vm.NewObject()
	if err := module.Set("exports", exports); err != nil {
		t.Fatalf("setting module exports: %v", err)
	}
	Require(context.Background(), options, adapter)(vm, module)
	if err := vm.Set("userk8s", exports); err != nil {
		t.Fatalf("exposing the module: %v", err)
	}

	resultCh := make(chan goja.Value, 1)
	errCh := make(chan error, 1)
	if err := vm.Set("__collect", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0)
		return goja.Undefined()
	}); err != nil {
		t.Fatalf("wiring __collect: %v", err)
	}
	if err := vm.Set("__collectErr", func(call goja.FunctionCall) goja.Value {
		errCh <- errors.New(call.Argument(0).String())
		return goja.Undefined()
	}); err != nil {
		t.Fatalf("wiring __collectErr: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	t.Cleanup(func() {
		_ = loop.Shutdown(context.Background())
		select {
		case <-runDone:
		case <-time.After(5 * time.Second):
		}
	})

	runAsync := func(body string) (goja.Value, error) {
		t.Helper()
		// The runtime belongs to the event loop. Submitting the script keeps
		// every goja touch on the loop goroutine; running it from the test
		// goroutine races the loop's own execution of pending jobs.
		runErr := make(chan error, 1)
		if err := loop.Submit(func() {
			_, err := vm.RunString("(async () => {" + body + "})().catch(e => __collectErr(String(e && e.code ? e.code + \": \" + e.message : e)))")
			runErr <- err
		}); err != nil {
			return goja.Undefined(), err
		}
		select {
		case err := <-runErr:
			if err != nil {
				return goja.Undefined(), err
			}
		case <-time.After(10 * time.Second):
			// A wedged loop must report like the result wait below, not hang
			// until the package timeout.
			return goja.Undefined(), errors.New("timeout submitting script to event loop")
		}
		select {
		case val := <-resultCh:
			return val, nil
		case err := <-errCh:
			return goja.Undefined(), err
		case <-time.After(10 * time.Second):
			return goja.Undefined(), errors.New("timeout waiting for async result")
		}
	}
	return vm, runAsync
}

func TestModuleLoadsCatalogFromArtifacts(t *testing.T) {
	_, runAsync := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}})
	value, err := runAsync(`
		const loaded = await userk8s.load();
		__collect(({
			source: loaded.source,
			countsOk: loaded.providers.length === 13 && loaded.models.length === 156
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
		}))
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
	_, runAsync := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}})
	value, err := runAsync(`
		const projection = userk8s.project("claude", "electronhub", "glm-5.3:dev");
		__collect(({
			providerSlug: projection.providerSlug,
			modelSlug: projection.modelSlug,
			budgetProfile: projection.budgetProfile,
			access: projection.access,
			surfacesOk: projection.surfaces.length > 0,
			numbersOk: projection.settings.context_window === 262000
				&& projection.settings.max_output_tokens === 16384
				&& projection.settings.can_reason === true
		}))
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

func TestModuleResolveCredentialRejectsTypedCode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	vm, runAsync := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}, Runner: failingRunner{}})
	_ = vm
	// With no resolvable credentials the resolution is missingCredentials;
	// the async contract rejects with the typed code and preserves the
	// documented status and slot-naming reason — never resolver output.
	_, err := runAsync(`
		await userk8s.resolveCredential("electronhub-shaper");
		__collect("UNEXPECTED-RESOLVE");
	`)
	if err == nil {
		t.Fatal("resolveCredential with failing runner: want a rejection")
	}
	if !strings.Contains(err.Error(), "credential-unresolved") {
		t.Errorf("rejection = %v, want the credential-unresolved code", err)
	}
	if strings.Contains(err.Error(), "test runner never resolves") {
		t.Errorf("rejection leaked raw resolver error text: %v", err)
	}
}

// failingOnceRunner fails every resolver execution with a raw error whose
// text (a fake resolver stdout/env detail) must never reach JavaScript.
type failingOnceRunner struct{}

const rawResolverSecret = "resolver-stdout: token=hunter2 env=AWS_SESSION_TOKEN=..."

func (failingOnceRunner) Run(context.Context, []string, time.Duration) (string, error) {
	return "", errors.New(rawResolverSecret)
}

// TestModuleResolveCredentialRedactsResolverFailure drives resolver failure
// redaction: the FilesBackend reports a failed command attempt through its
// sanitized attempts detail (status missingCredentials), so the module rejects
// with credential-unresolved — and by construction the raw resolver output
// (stdout/env text) never reaches JavaScript, whichever typed code the path
// produces.
func TestModuleResolveCredentialRedactsResolverFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, runAsync := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}, Runner: failingOnceRunner{}})
	_, err := runAsync(`
		await userk8s.resolveCredential("electronhub-shaper");
		__collect("UNEXPECTED-RESOLVE");
	`)
	if err == nil {
		t.Fatal("resolveCredential with failing resolver: want a rejection")
	}
	if !strings.Contains(err.Error(), "credential-unresolved") {
		t.Errorf("rejection = %v, want the credential-unresolved code", err)
	}
	if strings.Contains(err.Error(), rawResolverSecret) {
		t.Errorf("rejection leaked raw resolver output: %v", err)
	}
}

func TestModuleBackendStatus(t *testing.T) {
	_, runAsync := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}})
	value, err := runAsync(`
		const status = userk8s.backendStatus();
		__collect(({
			source: status.source,
			ok: status.providers === 13 && status.models === 156 && status.bindings === 15 && status.artifacts.length === 1
		}))
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
	_, runAsync := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}})
	_, err := runAsync(`
		try {
			await userk8s.resolveCredential("");
			__collectErr("empty reference resolved unexpectedly");
			return;
		} catch (e) {
			if (!(e instanceof TypeError)) { __collectErr("empty reference: want TypeError, got " + e); return; }
		}
		try {
			userk8s.project("claude", "electronhub", "");
			__collectErr("empty model projected unexpectedly");
			return;
		} catch (e) {
			// project stays a synchronous throw (pure, no promise machinery).
		}
		try {
			await userk8s.resolveCredential("no-such-access");
			__collectErr("unknown access resolved unexpectedly");
			return;
		} catch (e) {
			if (!e.code || (e.code !== "access-not-found" && e.code !== "credential-unresolved")) {
				__collectErr("unknown access: want a typed code, got " + (e.code || e));
				return;
			}
		}
		__collect("ALL-REJECTIONS-OK");
	`)
	if err != nil {
		t.Fatalf("rejection surface: %v", err)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestModuleProjectsReconciledSlug(t *testing.T) {
	_, runAsync := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}})
	value, err := runAsync(`
		const projection = userk8s.project("opencode", "electronhub", "glm-5.3:dev");
		__collect(({
			providerSlug: projection.providerSlug,
			modelSlug: projection.modelSlug,
			modelId: projection.modelId,
			budgetProfile: projection.budgetProfile,
			settingsOk: projection.settings.context_window === 262000 && projection.settings.max_output_tokens === 16384
		}))
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
	vm, runAsync := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}, Runner: resolvingRunner{}})
	_ = vm
	value, err := runAsync(`
		const resolution = await userk8s.resolveCredential("electronhub-shaper");
		const credential = (resolution.credentials || [])[0];
		__collect(({
			status: resolution.status,
			envVar: credential && credential.envVar,
			valuePresent: !!credential && credential.value.length > 0,
			provenanceOk: !!credential && credential.provenance.indexOf("command:") === 0
		}))`)
	_ = value
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
			_, runAsync := newRuntime(t, test.options)
			_, err := runAsync(`
				await userk8s.load();
				__collect("UNEXPECTED-LOAD");
			`)
			if err == nil {
				t.Fatalf("load(): want a rejection for %s", test.name)
			}
			if !strings.Contains(err.Error(), "catalog-not-found") {
				t.Errorf("rejection = %v, want the catalog-not-found code", err)
			}
		})
	}
}

// slowRunner delays before answering, so an abort can land mid-resolution.
type slowRunner struct{}

func (slowRunner) Run(ctx context.Context, _ []string, _ time.Duration) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(2 * time.Second):
		return "slow-value", nil
	}
}

// TestModuleResolveCredentialAbortsOnSignal proves the optional AbortSignal
// aborts a pending resolution: the promise rejects with ABORT_ERR / AbortError
// and the underlying resolver's context is cancelled (slowRunner returns on
// ctx.Done rather than completing its 2s wait).
func TestModuleResolveCredentialAbortsOnSignal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	vm, runAsync := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}, Runner: slowRunner{}})
	_ = vm
	start := time.Now()
	_, err := runAsync(`
		const controller = new AbortController();
		const pending = userk8s.resolveCredential("electronhub-shaper", controller.signal);
		controller.abort();
		try {
			await pending;
			__collectErr("UNEXPECTED-RESOLVE");
		} catch (e) {
			if (!e.code || (e.code !== "ABORT_ERR" && e.name !== "AbortError")) {
				__collectErr("want an AbortError / ABORT_ERR, got " + (e.code || e.name || e));
				return;
			}
			__collect("ABORTED-OK");
		}
	`)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("aborted resolveCredential: %v", err)
	}
	if elapsed >= 2*time.Second {
		t.Errorf("abort took %v; the resolver was not cancelled by the signal", elapsed)
	}
}

// TestModuleLoadIsAsync proves load() returns a promise: calling .then on it
// resolves with the snapshot, and the synchronous style of the old API no
// longer applies (load() itself returns a promise object, not the snapshot).
func TestModuleLoadIsAsync(t *testing.T) {
	_, runAsync := newRuntime(t, Options{Source: SourceFiles, Artifacts: []string{renderedProfile}})
	value, err := runAsync(`
		const pending = userk8s.load();
		const isPromise = typeof pending.then === "function";
		const loaded = await pending;
		__collect({ isPromise: isPromise, source: loaded.source, providers: loaded.providers.length });
	`)
	if err != nil {
		t.Fatalf("load(): %v", err)
	}
	got := value.Export().(map[string]any)
	if got["isPromise"] != true {
		t.Errorf("load() isPromise: got %v, want true", got["isPromise"])
	}
	if got["source"] != "files" {
		t.Errorf("load() source: got %v, want files", got["source"])
	}
}
