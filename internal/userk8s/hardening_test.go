package userk8s

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	projection, err := backend.Project(context.Background(), "claude", "electronhub", "glm-5.3:dev")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if projection.ProviderSlug != "electronhub" || projection.ModelSlug != "glm-5.3:dev" {
		t.Fatalf("slugs: got %q/%q", projection.ProviderSlug, projection.ModelSlug)
	}
	if projection.BudgetProfile != "claude-margin" {
		t.Fatalf("budget profile: got %q, want claude-margin", projection.BudgetProfile)
	}
	contextWindow, ok := projection.Settings["context_window"].(int64)
	if !ok {
		t.Fatalf("settings.context_window: got %T", projection.Settings["context_window"])
	}
	if contextWindow != 262000 {
		t.Fatalf("settings.context_window: got %d, want the registry fact 262000", contextWindow)
	}
	if applied := contextWindow - 2000; applied != 260000 {
		t.Fatalf("claude-margin derivation: got %d, want the documented 260000", applied)
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
	projection, err := backend.Project(context.Background(), "codex", "wafer", "GLM-5.1")
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if projection.BudgetProfile != "sdk-limits" {
		t.Fatalf("budget profile: got %q, want sdk-limits", projection.BudgetProfile)
	}
	if projection.ModelSlug != "GLM-5.1" {
		t.Fatalf("model slug: got %q, want the registry spelling GLM-5.1", projection.ModelSlug)
	}
	if got := projection.Settings["max_output_tokens"]; got != int64(32768) {
		t.Fatalf("settings.max_output_tokens: got %v, want the enforced cap 32768", got)
	}
	if got := projection.Settings["context_window"]; got != int64(203000) {
		t.Fatalf("settings.context_window: got %v, want 203000", got)
	}
}

func TestProjectResolvesRegistrySpelling(t *testing.T) {
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
  annotations:
    one-shot-man/registry-name: alpha-model:dev
  name: alpha-model-dev
spec:
  provider: alpha
  access: alpha-direct
  context_window: 262000
  max_output_tokens: 16384
`,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-direct
spec:
  provider: alpha
  auth:
    scheme: none
  endpoints:
    chat: http://127.0.0.1:9/alpha/v1
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
	byMetadata, err := backend.Project(context.Background(), "claude", "alpha", "alpha-model-dev")
	if err != nil {
		t.Fatalf("Project(metadata spelling): %v", err)
	}
	if byMetadata.ModelSlug != "alpha-model:dev" {
		t.Fatalf("model slug: got %q, want the registry spelling", byMetadata.ModelSlug)
	}

	registry, err := backend.Model("alpha-model:dev")
	if err != nil {
		t.Fatalf("Model(registry spelling): %v", err)
	}
	metadata, err := backend.Model("alpha-model-dev")
	if err != nil {
		t.Fatalf("Model(metadata spelling): %v", err)
	}
	if registry != metadata {
		t.Fatal("Model: both spellings must resolve to the same object")
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
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-mystery
spec:
  provider: alpha
  auth:
    scheme: mystery
`)
	backend := mustBackend(t, objects, &fakeRunner{})
	resolution, err := backend.ResolveCredential(context.Background(), "alpha-mystery")
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

func TestBindingSelectionIsOrderedByName(t *testing.T) {
	objects := documents(t,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-direct
  labels:
    one-shot-man/provider: alpha
spec:
  provider: alpha
  auth:
    scheme: bearer
    requiredEnv:
      - ALPHA_API_KEY
`,
		`apiVersion: one-shot-man/v1alpha1
kind: LocalSecretBinding
metadata:
  name: bind-b
spec:
  selector:
    matchLabels:
      one-shot-man/provider: alpha
  secretRef:
    name: creds
    key: ALPHA_API_KEY
  envVar: ALPHA_API_KEY
  resolvers:
    - command:
        argv:
          - cmd-b
`,
		`apiVersion: one-shot-man/v1alpha1
kind: LocalSecretBinding
metadata:
  name: bind-a
spec:
  selector:
    matchLabels:
      one-shot-man/provider: alpha
  secretRef:
    name: creds
    key: ALPHA_API_KEY
  envVar: ALPHA_API_KEY
  resolvers:
    - command:
        argv:
          - cmd-a
`)
	runner := &recordingRunner{values: map[string]string{"cmd-b": "from-b"}}
	resolution, err := mustBackend(t, objects, runner).ResolveCredential(context.Background(), "alpha-direct")
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if resolution.Status != StatusResolved {
		t.Fatalf("status: got %q (%s), want resolved", resolution.Status, resolution.Reason)
	}
	if got := resolution.Credentials[0].Value; got != "from-b" {
		t.Fatalf("value: got %q, want the value from bind-b", got)
	}
	if len(runner.calls) != 2 || runner.calls[0] != "cmd-a" || runner.calls[1] != "cmd-b" {
		t.Fatalf("calls: got %v, want bind-a before bind-b regardless of document order", runner.calls)
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
kind: ModelAccess
metadata:
  name: alpha-direct
  labels:
    one-shot-man/provider: alpha
spec:
  provider: alpha
  auth:
    scheme: bearer
    requiredEnv:
      - ALPHA_API_KEY
`,
		`apiVersion: one-shot-man/v1alpha1
kind: LocalSecretBinding
metadata:
  name: bind-alpha
spec:
  selector:
    matchLabels:
      one-shot-man/provider: alpha
  secretRef:
    name: creds
    key: ALPHA_API_KEY
  envVar: ALPHA_API_KEY
  resolvers:
    - command:
        argv: []
`)
	resolution, err := mustBackend(t, objects, &fakeRunner{}).ResolveCredential(context.Background(), "alpha-direct")
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

func TestCanceledContextIsReportedAsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	objects := documents(t,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-direct
spec:
  provider: alpha
  auth:
    scheme: bearer
    requiredEnv:
      - ALPHA_API_KEY
`)
	if _, err := mustBackend(t, objects, &fakeRunner{}).ResolveCredential(ctx, "alpha-direct"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveCredential: got %v, want context.Canceled", err)
	}

	_, err := (ExecRunner{}).Run(ctx, []string{"sleep", "10"}, time.Minute)
	if err == nil {
		t.Fatal("ExecRunner.Run: want an error for a canceled context")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("ExecRunner.Run: got %v, want a cancellation message rather than a timeout", err)
	}
	if strings.Contains(err.Error(), "timed out") {
		t.Fatalf("ExecRunner.Run: got %v, a canceled command must not be reported as a timeout", err)
	}
}

func TestResolverOutputIsBounded(t *testing.T) {
	buffer := &limitedBuffer{limit: 16}
	written, err := buffer.Write([]byte(strings.Repeat("x", 1024)))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if written != 1024 {
		t.Fatalf("Write: got %d, want the full length so the child never sees a short write", written)
	}
	if got := len(buffer.String()); got != 16 {
		t.Fatalf("buffer length: got %d, want the 16-byte limit", got)
	}
	if !buffer.truncated {
		t.Fatal("buffer: want truncation recorded")
	}

	counter := &countingWriter{limit: 8}
	if _, err := counter.Write([]byte(strings.Repeat("y", 4096))); err != nil {
		t.Fatalf("countingWriter.Write: %v", err)
	}
	if counter.Count() != 8 {
		t.Fatalf("countingWriter.Count: got %d, want the 8-byte limit", counter.Count())
	}
}

func TestReadCredentialFileUsesTheFirstLineOnly(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/token"
	if err := os.WriteFile(path, []byte("first\nsecond\nthird\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	value, err := readCredentialFile(path)
	if err != nil {
		t.Fatalf("readCredentialFile: %v", err)
	}
	if value != "first" {
		t.Fatalf("value: got %q, want the first line", value)
	}
}

func TestExpandHomeOnlyRewritesExactPrefixes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got, want := expandHome("~/.config/token"), home+"/.config/token"; got != want {
		t.Fatalf("expandHome(~/.config/token): got %q, want %q", got, want)
	}
	if got, want := expandHome("$HOME/.config/token"), home+"/.config/token"; got != want {
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

func TestServedSurfacesIntersectsToolAndAccess(t *testing.T) {
	access := &v1alpha1.ModelAccess{Spec: v1alpha1.ModelAccessSpec{
		Provider:  "alpha",
		Endpoints: map[string]v1alpha1.SurfaceEndpoint{"chat": "http://127.0.0.1:9/chat", "anthropic": "http://127.0.0.1:9/anthropic"},
	}}
	tool := &v1alpha1.Tool{Spec: v1alpha1.ToolSpec{
		DisplayName:        "OpenCode",
		Surfaces:           []string{"anthropic", "chat", "responses"},
		CredentialChannels: []string{"gateway"},
		Identifiers: v1alpha1.ToolIdentifiers{
			ProviderSlug: v1alpha1.ToolIdentifierGrammar{Pattern: `^[a-z]+$`},
			ModelSlug:    v1alpha1.ToolIdentifierGrammar{Pattern: `^[a-z]+$`},
		},
		BudgetProfile: "sdk-limits",
	}}
	got := servedSurfaces(access, tool)
	if len(got) != 2 || got[0] != "anthropic" || got[1] != "chat" {
		t.Fatalf("servedSurfaces: got %v, want the sorted intersection [anthropic chat]", got)
	}
	if _, err := selectorMatches(&metav1.LabelSelector{}, labels.Set{}); err != nil {
		t.Fatalf("selectorMatches(empty selector): %v", err)
	}
}
