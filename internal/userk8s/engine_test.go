package userk8s

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// fakeRunner is a CommandRunner that returns canned results and records calls.
type fakeRunner struct {
	steps []fakeStep
	calls []fakeCall
}

type fakeStep struct {
	argv     []string
	stdout   string
	stderr   string
	err      error
	deadline bool
}

type fakeCall struct {
	argv    []string
	timeout time.Duration
}

func (f *fakeRunner) Run(ctx context.Context, argv []string, timeout time.Duration) (string, error) {
	f.calls = append(f.calls, fakeCall{argv: argv, timeout: timeout})
	if timeout <= 0 {
		return "", errors.New("resolver command must be given a positive timeout")
	}
	for _, step := range f.steps {
		if strings.Join(step.argv, "\x00") != strings.Join(argv, "\x00") {
			continue
		}
		if step.deadline {
			<-ctx.Done()
			return "", ctx.Err()
		}
		return step.stdout, step.err
	}
	return "", errors.New("fake runner has no result for the command")
}

func documents(t *testing.T, bodies ...string) *Objects {
	t.Helper()
	raw := make([][]byte, 0, len(bodies))
	for _, body := range bodies {
		raw = append(raw, []byte(body))
	}
	objects, err := LoadDocuments(raw...)
	if err != nil {
		t.Fatalf("LoadDocuments: %v", err)
	}
	return objects
}

const fixture = "testdata/rendered-personal.yaml"

func mustBackend(t *testing.T, objects *Objects, runner CommandRunner) *FilesBackend {
	t.Helper()
	backend, err := NewFilesBackendFromObjects(objects, runner)
	if err != nil {
		t.Fatalf("NewFilesBackendFromObjects: %v", err)
	}
	return backend
}

func TestLoadRenderedProfileFixture(t *testing.T) {
	objects, err := LoadPaths(fixture)
	if err != nil {
		t.Fatalf("LoadPaths(%s): %v", fixture, err)
	}
	for _, want := range []struct {
		name string
		got  int
		want int
	}{
		{"providers", len(objects.Providers), 2},
		{"accesses", len(objects.Accesses), 2},
		{"models", len(objects.Models), 2},
		{"tools", len(objects.Tools), 2},
		{"bindings", len(objects.Bindings), 2},
	} {
		if want.got != want.want {
			t.Errorf("%s: got %d, want %d", want.name, want.got, want.want)
		}
	}
	if len(objects.Artifacts) != 1 {
		t.Errorf("artifacts: got %v, want one entry", objects.Artifacts)
	}
}

func TestLoadSkipsNonCatalogKinds(t *testing.T) {
	objects := documents(t,
		`apiVersion: v1
kind: Namespace
metadata:
  name: model-access-test
`,
		`apiVersion: v1
kind: Secret
metadata:
  name: creds
type: Opaque
stringData:
  ALPHA_API_KEY: shh
`,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`)
	if objects.Skipped != 2 {
		t.Fatalf("skipped: got %d, want 2", objects.Skipped)
	}
	if len(objects.Providers) != 1 {
		t.Fatalf("providers: got %d, want 1", len(objects.Providers))
	}
}

func TestLoadRejectsInvalidDocuments(t *testing.T) {
	provider := `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`
	tests := []struct {
		name    string
		bodies  []string
		wantErr string
	}{
		{
			name: "unknown apiVersion",
			bodies: []string{`apiVersion: config.joeyc.ai/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`},
			wantErr: "want \"one-shot-man/v1alpha1\"",
		},
		{
			name: "unknown kind",
			bodies: []string{`apiVersion: one-shot-man/v1alpha1
kind: NotACatalogKind
metadata:
  name: alpha
`},
			wantErr: "not a known catalog kind",
		},
		{
			name: "missing kind",
			bodies: []string{`apiVersion: one-shot-man/v1alpha1
metadata:
  name: alpha
`},
			wantErr: "document has no kind",
		},
		{
			name:    "duplicate name",
			bodies:  []string{provider, provider},
			wantErr: "duplicate ModelProvider",
		},
		{
			name: "access with undeclared provider",
			bodies: []string{`apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: ghost-direct
spec:
  provider: ghost
  auth:
    scheme: bearer
    requiredEnv:
      - GHOST_API_KEY
`},
			wantErr: "references undeclared ModelProvider \"ghost\"",
		},
		{
			name: "model whose access belongs to another provider",
			bodies: []string{provider,
				`apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: beta
spec:
  display_name: Beta
`,
				`apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-direct
spec:
  provider: alpha
  auth:
    scheme: none
`,
				`apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-model
spec:
  provider: beta
  access: alpha-direct
  context_window: 1000
  max_output_tokens: 10
`},
			wantErr: "belongs to provider \"alpha\"",
		},
		{
			name: "duplicate default per provider",
			bodies: []string{provider,
				`apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-one
spec:
  provider: alpha
  context_window: 1000
  max_output_tokens: 10
  default: true
`,
				`apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-two
spec:
  provider: alpha
  context_window: 1000
  max_output_tokens: 10
  default: true
`},
			wantErr: "both declare default",
		},
		{
			name: "asymmetric equivalence",
			bodies: []string{provider,
				`apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-one
spec:
  provider: alpha
  context_window: 1000
  max_output_tokens: 10
  equivalent_to:
    - provider: alpha
      name: alpha-two
`,
				`apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-two
spec:
  provider: alpha
  context_window: 1000
  max_output_tokens: 10
`},
			wantErr: "must be symmetric",
		},
		{
			name: "empty binding selector",
			bodies: []string{`apiVersion: one-shot-man/v1alpha1
kind: LocalSecretBinding
metadata:
  name: bind-anything
spec:
  selector: {}
  secretRef:
    name: creds
    key: ALPHA_API_KEY
  envVar: ALPHA_API_KEY
`},
			wantErr: "empty selector",
		},
		{
			name: "uncompilable tool grammar",
			bodies: []string{`apiVersion: one-shot-man/v1alpha1
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
      pattern: "("
    model_slug:
      pattern: ^[a-z]+$
  budget_profile: claude-margin
`},
			wantErr: "identifiers.provider_slug",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadDocuments(toBytes(test.bodies)...)
			if err == nil {
				t.Fatalf("LoadDocuments: want error containing %q, got nil", test.wantErr)
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("LoadDocuments: got error %q, want it to contain %q", err, test.wantErr)
			}
		})
	}
}

func toBytes(bodies []string) [][]byte {
	raw := make([][]byte, 0, len(bodies))
	for _, body := range bodies {
		raw = append(raw, []byte(body))
	}
	return raw
}

func TestSelectorConformance(t *testing.T) {
	set := labels.Set{
		"one-shot-man/provider":          "alpha",
		"one-shot-man/mode":              "direct",
		"one-shot-man/surface-chat":      "true",
		"one-shot-man/surface-anthropic": "true",
	}
	tests := []struct {
		name     string
		selector metav1.LabelSelector
		want     bool
	}{
		{
			name:     "matchLabels",
			selector: metav1.LabelSelector{MatchLabels: map[string]string{"one-shot-man/provider": "alpha"}},
			want:     true,
		},
		{
			name:     "matchLabels mismatch",
			selector: metav1.LabelSelector{MatchLabels: map[string]string{"one-shot-man/provider": "beta"}},
			want:     false,
		},
		{
			name: "In matches",
			selector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "one-shot-man/mode",
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"direct", "proxy"},
			}}},
			want: true,
		},
		{
			name: "In rejects an unlisted value",
			selector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "one-shot-man/mode",
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"shaper"},
			}}},
			want: false,
		},
		{
			name: "NotIn matches an unlisted value",
			selector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "one-shot-man/mode",
				Operator: metav1.LabelSelectorOpNotIn,
				Values:   []string{"shaper"},
			}}},
			want: true,
		},
		{
			name: "NotIn rejects a listed value",
			selector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "one-shot-man/mode",
				Operator: metav1.LabelSelectorOpNotIn,
				Values:   []string{"direct"},
			}}},
			want: false,
		},
		{
			name: "Exists matches",
			selector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "one-shot-man/surface-chat",
				Operator: metav1.LabelSelectorOpExists,
			}}},
			want: true,
		},
		{
			name: "Exists rejects an absent key",
			selector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "one-shot-man/surface-responses",
				Operator: metav1.LabelSelectorOpExists,
			}}},
			want: false,
		},
		{
			name: "DoesNotExist matches",
			selector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "one-shot-man/surface-responses",
				Operator: metav1.LabelSelectorOpDoesNotExist,
			}}},
			want: true,
		},
		{
			name: "DoesNotExist rejects a present key",
			selector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      "one-shot-man/surface-chat",
				Operator: metav1.LabelSelectorOpDoesNotExist,
			}}},
			want: false,
		},
		{
			name: "annotations and expressions AND together",
			selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"one-shot-man/provider": "alpha"},
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "one-shot-man/surface-responses",
					Operator: metav1.LabelSelectorOpExists,
				}},
			},
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := selectorMatches(&test.selector, set)
			if err != nil {
				t.Fatalf("selectorMatches: %v", err)
			}
			if got != test.want {
				t.Fatalf("selectorMatches: got %v, want %v", got, test.want)
			}
		})
	}
}

func TestFilesBackendResolvesCredentials(t *testing.T) {
	backend, err := NewFilesBackend(FilesBackendOptions{Paths: []string{fixture}})
	if err != nil {
		t.Fatalf("NewFilesBackend: %v", err)
	}
	ctx := context.Background()

	t.Run("env resolver wins", func(t *testing.T) {
		t.Setenv("SHELL_AI_ALPHA_API_KEY", "alpha-env-value")
		backend.runner = &fakeRunner{}
		resolution, err := backend.ResolveCredential(ctx, "alpha-direct")
		if err != nil {
			t.Fatalf("ResolveCredential: %v", err)
		}
		if resolution.Status != StatusResolved {
			t.Fatalf("status: got %q (%s), want %q", resolution.Status, resolution.Reason, StatusResolved)
		}
		if len(resolution.Credentials) != 1 {
			t.Fatalf("credentials: got %+v, want exactly one", resolution.Credentials)
		}
		credential := resolution.Credentials[0]
		if credential.EnvVar != "ALPHA_API_KEY" || credential.Value != "alpha-env-value" {
			t.Fatalf("credential: got %+v", credential)
		}
		if credential.Provenance != "env:SHELL_AI_ALPHA_API_KEY" {
			t.Fatalf("provenance: got %q", credential.Provenance)
		}
	})

	t.Run("command resolver supplies the value when env and file miss", func(t *testing.T) {
		os.Unsetenv("SHELL_AI_ALPHA_API_KEY")
		runner := &fakeRunner{steps: []fakeStep{{
			argv:   []string{"fake-op", "item", "get", "alpha"},
			stdout: "alpha-command-value\ntrailing\n",
		}}}
		backend.runner = runner
		resolution, err := backend.ResolveCredential(ctx, "alpha-direct")
		if err != nil {
			t.Fatalf("ResolveCredential: %v", err)
		}
		if resolution.Status != StatusResolved {
			t.Fatalf("status: got %q (%s)", resolution.Status, resolution.Reason)
		}
		if got := resolution.Credentials[0].Value; got != "alpha-command-value" {
			t.Fatalf("value: got %q, want the first line only", got)
		}
		if got := resolution.Credentials[0].Provenance; got != "command:fake-op" {
			t.Fatalf("provenance: got %q", got)
		}
		if len(runner.calls) != 1 {
			t.Fatalf("calls: got %+v, want one", runner.calls)
		}
		if runner.calls[0].timeout != 60*time.Second {
			t.Fatalf("timeout: got %s, want the declared 60s", runner.calls[0].timeout)
		}
	})

	t.Run("file resolver provides the first line", func(t *testing.T) {
		os.Unsetenv("SHELL_AI_BETA_API_KEY")
		dir := t.TempDir()
		path := filepath.Join(dir, "beta.token")
		if err := os.WriteFile(path, []byte("beta-file-value\nignored\n"), 0o600); err != nil {
			t.Fatalf("writing the credential file: %v", err)
		}
		objects := documents(t,
			`apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: beta
spec:
  display_name: Beta
`,
			`apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: beta-direct
  labels:
    one-shot-man/provider: beta
spec:
  provider: beta
  auth:
    scheme: bearer
    requiredEnv:
      - BETA_API_KEY
`,
			`apiVersion: one-shot-man/v1alpha1
kind: LocalSecretBinding
metadata:
  name: bind-beta
spec:
  selector:
    matchLabels:
      one-shot-man/provider: beta
  secretRef:
    name: creds
    key: BETA_API_KEY
  envVar: BETA_API_KEY
  resolvers:
    - file:
        path: `+path+`
`)
		local := mustBackend(t, objects, &fakeRunner{})
		resolution, err := local.ResolveCredential(ctx, "beta-direct")
		if err != nil {
			t.Fatalf("ResolveCredential: %v", err)
		}
		if resolution.Status != StatusResolved {
			t.Fatalf("status: got %q (%s)", resolution.Status, resolution.Reason)
		}
		if got := resolution.Credentials[0].Value; got != "beta-file-value" {
			t.Fatalf("value: got %q, want the trimmed first line", got)
		}
	})

	t.Run("missing binding reports every uncovered slot", func(t *testing.T) {
		backend.runner = &fakeRunner{}
		resolution, err := backend.ResolveCredential(ctx, "beta-shaper")
		if err != nil {
			t.Fatalf("ResolveCredential: %v", err)
		}
		if resolution.Status != StatusMissingCredentials {
			t.Fatalf("status: got %q, want %q", resolution.Status, StatusMissingCredentials)
		}
		if !strings.Contains(resolution.Reason, `"BETA_API_KEY"`) {
			t.Fatalf("reason: got %q, want it to name the uncovered slot", resolution.Reason)
		}
		if len(resolution.Credentials) != 0 {
			t.Fatalf("credentials: got %+v, want none", resolution.Credentials)
		}
	})

	t.Run("scheme none resolves without credentials", func(t *testing.T) {
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
  name: alpha-loopback
spec:
  provider: alpha
  auth:
    scheme: none
`)
		local := mustBackend(t, objects, &fakeRunner{})
		resolution, err := local.ResolveCredential(ctx, "alpha-loopback")
		if err != nil {
			t.Fatalf("ResolveCredential: %v", err)
		}
		if resolution.Status != StatusResolved {
			t.Fatalf("status: got %q (%s)", resolution.Status, resolution.Reason)
		}
	})

	t.Run("launch-gated schemes report precise reasons", func(t *testing.T) {
		tests := []struct {
			scheme      string
			requiredEnv string
		}{
			{scheme: "awsSigV4", requiredEnv: "    requiredEnv:\n      - AWS_ACCESS_KEY_ID\n      - AWS_SECRET_ACCESS_KEY\n"},
			{scheme: "toolManaged"},
		}
		for _, test := range tests {
			scheme := test.scheme
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
  name: alpha-`+scheme+`
spec:
  provider: alpha
  auth:
    scheme: `+scheme+`
`+test.requiredEnv)
			local := mustBackend(t, objects, &fakeRunner{})
			resolution, err := local.ResolveCredential(ctx, "alpha-"+scheme)
			if err != nil {
				t.Fatalf("ResolveCredential(%s): %v", scheme, err)
			}
			if resolution.Status != StatusUnsupportedScheme {
				t.Fatalf("%s status: got %q, want %q", scheme, resolution.Status, StatusUnsupportedScheme)
			}
			if !strings.Contains(resolution.Reason, scheme) {
				t.Fatalf("%s reason: got %q, want it to name the scheme", scheme, resolution.Reason)
			}
		}
	})
}

func TestResolveCredentialNeverLeaksValues(t *testing.T) {
	const secret = "super-secret-token-value"
	runner := &fakeRunner{steps: []fakeStep{{
		argv:   []string{"fake-op", "item", "get", "alpha"},
		stderr: secret,
		err:    errors.New("boom: " + secret),
	}}}
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
        argv:
          - fake-op
          - item
          - get
          - alpha
`)
	backend := mustBackend(t, objects, runner)
	resolution, err := backend.ResolveCredential(context.Background(), "alpha-direct")
	if err != nil {
		t.Fatalf("ResolveCredential: %v", err)
	}
	if resolution.Status != StatusMissingCredentials {
		t.Fatalf("status: got %q, want %q", resolution.Status, StatusMissingCredentials)
	}
	if strings.Contains(resolution.Reason, secret) {
		t.Fatalf("reason leaked the credential value: %q", resolution.Reason)
	}
}

func TestExecRunnerRedactsStderrAndHonorsTimeout(t *testing.T) {
	ctx := context.Background()

	t.Run("stderr is counted, never quoted", func(t *testing.T) {
		const secret = "leaked-secret-value"
		_, err := (ExecRunner{}).Run(ctx, []string{"sh", "-c", "echo " + secret + " >&2; exit 3"}, time.Second)
		if err == nil {
			t.Fatal("Run: want an error for a failing command")
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked stderr content: %v", err)
		}
	})

	t.Run("timeout is enforced and reported", func(t *testing.T) {
		start := time.Now()
		_, err := (ExecRunner{}).Run(ctx, []string{"sleep", "10"}, 50*time.Millisecond)
		if err == nil {
			t.Fatal("Run: want a timeout error")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("error: got %v, want a timeout message", err)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("elapsed %s: the timeout was not enforced", elapsed)
		}
	})
}

func TestProjectWorkedExamples(t *testing.T) {
	backend, err := NewFilesBackend(FilesBackendOptions{Paths: []string{fixture}})
	if err != nil {
		t.Fatalf("NewFilesBackend: %v", err)
	}
	ctx := context.Background()

	t.Run("claude-margin row projects registry facts for the adapter to derive", func(t *testing.T) {
		projection, err := backend.Project(ctx, "claude", "alpha", "alpha-model")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if projection.ProviderSlug != "alpha" || projection.ModelSlug != "alpha-model" {
			t.Fatalf("slugs: got %q/%q", projection.ProviderSlug, projection.ModelSlug)
		}
		if projection.BudgetProfile != "claude-margin" {
			t.Fatalf("budget profile: got %q", projection.BudgetProfile)
		}
		contextWindow, ok := projection.Settings["context_window"].(int64)
		if !ok {
			t.Fatalf("settings.context_window: got %T", projection.Settings["context_window"])
		}
		if contextWindow != 100000 {
			t.Fatalf("settings.context_window: got %d, want the registry fact 100000", contextWindow)
		}
	})

	t.Run("sdk-limits row carries the enforced output cap", func(t *testing.T) {
		projection, err := backend.Project(ctx, "opencode", "beta", "beta-model")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if projection.BudgetProfile != "sdk-limits" {
			t.Fatalf("budget profile: got %q", projection.BudgetProfile)
		}
		if got := projection.Settings["max_output_tokens"]; got != int64(2000) {
			t.Fatalf("settings.max_output_tokens: got %v", got)
		}
	})

	t.Run("tool overrides win over model facts", func(t *testing.T) {
		objects, err := LoadPaths(fixture)
		if err != nil {
			t.Fatalf("LoadPaths: %v", err)
		}
		model := objects.modelByName["alpha-model"]
		if model == nil {
			t.Fatal("fixture is missing alpha-model")
		}
		contextWindow, autoCompact := int64(999), int64(500)
		effort, waive := "high", "the tool caps below the provider fact"
		model.Spec.ToolSettings = map[string]v1alpha1.ModelToolSettings{
			"claude": {
				ContextWindow:     &contextWindow,
				AutoCompactWindow: &autoCompact,
				EffortLevel:       &effort,
				Flags:             map[string]string{"beta": "true"},
				Waive:             waive,
			},
		}
		backend := mustBackend(t, objects, &fakeRunner{})

		projection, err := backend.Project(ctx, "claude", "alpha", "alpha-model")
		if err != nil {
			t.Fatalf("Project: %v", err)
		}
		if got := projection.Settings["context_window"]; got != contextWindow {
			t.Fatalf("settings.context_window: got %v, want the override %d", got, contextWindow)
		}
		if got := projection.Settings["auto_compact_window"]; got != autoCompact {
			t.Fatalf("settings.auto_compact_window: got %v, want %d", got, autoCompact)
		}
		if got := projection.Settings["effort_level"]; got != effort {
			t.Fatalf("settings.effort_level: got %v, want %q", got, effort)
		}
		if got := projection.Settings["waive"]; got != waive {
			t.Fatalf("settings.waive: got %v, want %q", got, waive)
		}
		flags, ok := projection.Settings["flags"].(map[string]string)
		if !ok || flags["beta"] != "true" {
			t.Fatalf("settings.flags: got %v", projection.Settings["flags"])
		}

		unmodified, err := backend.Project(ctx, "opencode", "alpha", "alpha-model")
		if err != nil {
			t.Fatalf("Project(opencode): %v", err)
		}
		if got := unmodified.Settings["context_window"]; got != int64(100000) {
			t.Fatalf("settings.context_window for a tool without an override: got %v, want the model fact 100000", got)
		}
		if _, ok := unmodified.Settings["waive"]; ok {
			t.Fatalf("settings.waive should be absent for a tool without an override: %v", unmodified.Settings)
		}
	})

	t.Run("provider mismatch is rejected", func(t *testing.T) {
		_, err := backend.Project(ctx, "claude", "beta", "alpha-model")
		if err == nil {
			t.Fatal("Project: want an error for a mismatched provider")
		}
		if !strings.Contains(err.Error(), "belongs to provider") {
			t.Fatalf("error: got %v", err)
		}
	})

	t.Run("unknown tool is rejected", func(t *testing.T) {
		if _, err := backend.Project(ctx, "ghost", "alpha", "alpha-model"); err == nil {
			t.Fatal("Project: want an error for an unknown tool")
		}
	})
}
