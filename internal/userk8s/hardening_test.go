package userk8s

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/labels"
)

// The claude wrapper's applied context is the registry context minus the
// margin recorded in scratch/osm-ai-architecture/reconciliation.md (2000 for
// electronhub's 262000 rows); the catalog must expose the registry fact
// unchanged so that derivation stays deterministic in adapter code.
func TestProjectClaudeMarginWorkedExample(t *testing.T) {
	backend, err := NewFilesBackend(FilesBackendOptions{Paths: []string{renderedPersonalProfile}})
	if err != nil {
		t.Fatalf("NewFilesBackend: %v", err)
	}
	projection, err := backend.Project(context.Background(), "claude", "electronhub", "glm-5.3:dev:electronhub")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if projection.ProviderSlug != "electronhub" || projection.ModelSlug != "glm-5.3:dev:electronhub" {
		t.Fatalf("slugs: got %q/%q", projection.ProviderSlug, projection.ModelSlug)
	}
	if projection.BudgetProfile != "claude-margin" {
		t.Fatalf("budget profile: got %q, want claude-margin", projection.BudgetProfile)
	}
	contextWindow, ok := projection.Settings["context_window"].(int64)
	if !ok {
		t.Fatalf("settings.context_window: got %T", projection.Settings["context_window"])
	}
	if contextWindow != 260000 {
		t.Fatalf("settings.context_window: got %d, want the serving's effective fact 260000", contextWindow)
	}
	if applied := contextWindow - 2000; applied != 258000 {
		t.Fatalf("claude-margin derivation: got %d, want 258000", applied)
	}
	if got := projection.Settings["max_output_tokens"]; got != int64(16384) {
		t.Fatalf("settings.max_output_tokens: got %v, want 16384", got)
	}
}

// The wafer rows are sdk-limits: the projected output budget equals the
// model's enforced cap, with no derivation.
func TestProjectSDKLimitsWorkedExample(t *testing.T) {
	backend, err := NewFilesBackend(FilesBackendOptions{Paths: []string{renderedPersonalProfile}})
	if err != nil {
		t.Fatalf("NewFilesBackend: %v", err)
	}
	projection, err := backend.Project(context.Background(), "codex", "zhipu", "glm-5-1-zhipu")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if projection.BudgetProfile != "sdk-limits" {
		t.Fatalf("budget profile: got %q, want sdk-limits", projection.BudgetProfile)
	}
	if projection.ModelSlug != "glm_5_1_zhipu" {
		t.Fatalf("model slug: got %q, want the derived glm_5_1_zhipu", projection.ModelSlug)
	}
	if got := projection.Settings["max_output_tokens"]; got != int64(32768) {
		t.Fatalf("settings.max_output_tokens: got %v, want the enforced cap 32768", got)
	}
	if got := projection.Settings["context_window"]; got != int64(200000) {
		t.Fatalf("settings.context_window: got %v, want 200000", got)
	}
}

func TestProjectResolvesServingRegistrySpelling(t *testing.T) {
	objects := documents(t,
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
  context_window: 1000
  max_output_tokens: 10
`,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: alpha-model-alpha
  annotations:
    one-shot-man/registry-name: alpha-model:dev
spec:
  provider: alpha
  model: alpha-model
  provider_model_id: alpha-model:dev
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

	byRegistry, err := backend.Project(context.Background(), "claude", "alpha", "alpha-model:dev")
	if err != nil {
		t.Fatalf("Project(registry spelling): %v", err)
	}
	if byRegistry.ModelSlug != "alpha-model:dev" {
		t.Fatalf("model slug: got %q, want the registry spelling", byRegistry.ModelSlug)
	}
	byMetadata, err := backend.Project(context.Background(), "claude", "alpha", "alpha-model-alpha")
	if err != nil {
		t.Fatalf("Project(metadata spelling): %v", err)
	}
	if byMetadata.ModelSlug != "alpha-model:dev" {
		t.Fatalf("model slug: got %q, want the registry spelling", byMetadata.ModelSlug)
	}
}

func TestSelectorMatchingRejectsNilSelector(t *testing.T) {
	matched, err := selectorMatches(nil, labels.Set{"one-shot-man/provider": "alpha"})
	if err != nil {
		t.Fatalf("selectorMatches(nil): %v", err)
	}
	if matched {
		t.Fatal("selectorMatches(nil): want false, an absent selector must not match everything")
	}
}

func TestResolveCredentialUnknownScheme(t *testing.T) {
	objects := documents(t,
		`apiVersion: one-shot-man/v1alpha1
kind: LocalSecretBinding
metadata:
  name: bind-mystery
spec:
  selector:
    matchLabels:
      one-shot-man/provider: alpha
  scheme: mystery
  secretRef:
    name: creds
    key: ALPHA_API_KEY
  envVar: ALPHA_API_KEY
`)
	backend := mustBackend(t, objects, &fakeRunner{})
	resolution, err := backend.ResolveCredential(context.Background(), "bind-mystery")
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if resolution.Status != StatusUnsupportedScheme {
		t.Fatalf("status: got %q, want %q", resolution.Status, StatusUnsupportedScheme)
	}
	if !strings.Contains(resolution.Reason, "mystery") {
		t.Fatalf("reason: got %q, want it to name the scheme", resolution.Reason)
	}
}

type recordingRunner struct {
	calls  []string
	values map[string]string
}

func (r *recordingRunner) Run(_ context.Context, argv []string, _ time.Duration) (string, error) {
	r.calls = append(r.calls, argv[0])
	if value, ok := r.values[argv[0]]; ok {
		return value, nil
	}
	return "", errors.New("recording runner: no value")
}

func TestBindingsResolveIndependentlyByName(t *testing.T) {
	binding := func(name, cmd string) string {
		return `apiVersion: one-shot-man/v1alpha1
kind: LocalSecretBinding
metadata:
  name: ` + name + `
spec:
  selector:
    matchLabels:
      one-shot-man/provider: alpha
  scheme: bearer
  secretRef:
    name: creds
    key: ALPHA_API_KEY
  envVar: ALPHA_API_KEY
  resolvers:
    - command:
        argv:
          - ` + cmd + `
`
	}
	objects := documents(t, binding("bind-a", "cmd-a"), binding("bind-b", "cmd-b"))
	runner := &recordingRunner{values: map[string]string{"cmd-a": "from-a", "cmd-b": "from-b"}}
	backend := mustBackend(t, objects, runner)

	for _, test := range []struct{ name, want string }{{"bind-a", "from-a"}, {"bind-b", "from-b"}} {
		resolution, err := backend.ResolveCredential(context.Background(), test.name)
		if err != nil {
			t.Fatalf("ResolveCredential(%s): %v", test.name, err)
		}
		if resolution.Status != StatusResolved {
			t.Fatalf("ResolveCredential(%s): status %q (%s)", test.name, resolution.Status, resolution.Reason)
		}
		if got := resolution.Credentials[0].Value; got != test.want {
			t.Fatalf("ResolveCredential(%s): value %q, want %q", test.name, got, test.want)
		}
	}
	if len(runner.calls) != 2 || runner.calls[0] != "cmd-a" || runner.calls[1] != "cmd-b" {
		t.Fatalf("calls: got %v, want each binding resolved once in call order", runner.calls)
	}
}

func TestEmptyCommandArgvIsAResolverFailure(t *testing.T) {
	objects := documents(t,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`,
		`apiVersion: one-shot-man/v1alpha1
kind: LocalSecretBinding
metadata:
  name: bind-alpha
spec:
  selector:
    matchLabels:
      one-shot-man/provider: alpha
  scheme: bearer
  secretRef:
    name: creds
    key: ALPHA_API_KEY
  envVar: ALPHA_API_KEY
  resolvers:
    - command:
        argv: []
`)
	resolution, err := mustBackend(t, objects, &fakeRunner{}).ResolveCredential(context.Background(), "bind-alpha")
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if resolution.Status != StatusMissingCredentials {
		t.Fatalf("status: got %q, want %q", resolution.Status, StatusMissingCredentials)
	}
	if !strings.Contains(resolution.Reason, string(failureEmptyArgv)) {
		t.Fatalf("reason: got %q, want the empty-argv failure class", resolution.Reason)
	}
}

func TestExpandHomeOnlyRewritesExactPrefixes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	want := filepath.Join(home, ".config", "token")
	if got := expandHome("~/.config/token"); got != want {
		t.Fatalf("expandHome(~/.config/token): got %q, want %q", got, want)
	}
	if got := expandHome("$HOME/.config/token"); got != want {
		t.Fatalf("expandHome($HOME/.config/token): got %q, want %q", got, want)
	}
	if got := expandHome("~other/token"); strings.HasPrefix(got, home) {
		t.Fatalf("expandHome(~other/token): got %q, another user's home must not be rewritten", got)
	}
	if got := expandHome("$HOMEfoo/token"); strings.HasPrefix(got, home) {
		t.Fatalf("expandHome($HOMEfoo/token): got %q, a longer variable name must not be rewritten", got)
	}
}

func TestNewFilesBackendFromObjectsRejectsNil(t *testing.T) {
	if _, err := NewFilesBackendFromObjects(nil, &fakeRunner{}); err == nil {
		t.Fatal("NewFilesBackendFromObjects(nil): want an error rather than a nil dereference")
	}
}

func TestProjectDerivesToolIdentifiers(t *testing.T) {
	backend, err := NewFilesBackend(FilesBackendOptions{Paths: []string{renderedPersonalProfile}})
	if err != nil {
		t.Fatalf("NewFilesBackend: %v", err)
	}

	opencode, err := backend.Project(context.Background(), "opencode", "electronhub", "glm-5.3:dev:electronhub")
	if err != nil {
		t.Fatalf("Project(opencode): %v", err)
	}
	if opencode.ModelSlug != "glm_5_3_dev_electronhub" {
		t.Fatalf("opencode model slug: got %q, want the derived key glm_5_3_dev_electronhub", opencode.ModelSlug)
	}
	if opencode.ModelID != "glm-5.3:dev:electronhub" {
		t.Fatalf("opencode model id: got %q, want the raw registry spelling", opencode.ModelID)
	}

	claude, err := backend.Project(context.Background(), "claude", "electronhub", "glm-5.3:dev:electronhub")
	if err != nil {
		t.Fatalf("Project(claude): %v", err)
	}
	if claude.ModelSlug != "glm-5.3:dev:electronhub" || claude.ModelID != "glm-5.3:dev:electronhub" {
		t.Fatalf("claude identifiers: got slug %q id %q, want the raw spelling kept verbatim", claude.ModelSlug, claude.ModelID)
	}
}
