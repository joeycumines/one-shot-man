package userk8s

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
)

// Catalog is the read-only view of the model catalog every backend
// implements. Files and cluster backends are interchangeable: adapters and the
// scripting surface depend on this interface only.
type Catalog interface {
	ListProviders(ctx context.Context) ([]v1alpha1.ModelProvider, error)
	ListAccesses(ctx context.Context) ([]v1alpha1.ModelAccess, error)
	ListModels(ctx context.Context) ([]v1alpha1.Model, error)
	ListTools(ctx context.Context) ([]v1alpha1.Tool, error)

	// ResolveCredential resolves one access's credential requirements.
	ResolveCredential(ctx context.Context, accessRef string) (Resolution, error)

	// Project derives the tool-facing slug pair and settings for one
	// selection. Budget derivation is adapter code; the catalog only names
	// the tool's budget profile.
	Project(ctx context.Context, tool, provider, model string) (Projection, error)

	Status(ctx context.Context) (BackendStatus, error)
}

// FilesBackend serves the catalog from rendered profile artifacts on disk. It
// also carries the catalog logic every backend shares: the cluster backend
// serves the same objects and therefore reuses these methods verbatim.
type FilesBackend struct {
	objects *Objects
	runner  CommandRunner
	source  string
}

// Compile-time proof that the files backend satisfies the shared interface.
var _ Catalog = (*FilesBackend)(nil)

// FilesBackendOptions configures a FilesBackend.
type FilesBackendOptions struct {
	// Paths are the rendered artifact files or directories to load.
	Paths []string

	// Runner executes command resolvers; tests inject a fake.
	Runner CommandRunner
}

// NewFilesBackend loads the configured artifacts and returns a ready backend.
func NewFilesBackend(options FilesBackendOptions) (*FilesBackend, error) {
	if len(options.Paths) == 0 {
		return nil, errors.New("userk8s: files backend requires at least one artifact path")
	}
	objects, err := LoadPaths(options.Paths...)
	if err != nil {
		return nil, err
	}
	return NewFilesBackendFromObjects(objects, options.Runner)
}

// NewFilesBackendFromObjects wraps already-loaded objects; used by the cluster
// backend and by tests that share one fixture across both backends. Passing
// nil objects is rejected rather than deferred to a nil dereference.
func NewFilesBackendFromObjects(objects *Objects, runner CommandRunner) (*FilesBackend, error) {
	if objects == nil {
		return nil, errors.New("userk8s: files backend requires loaded objects")
	}
	return newBackendFromObjects(objects, runner, SourceFiles)
}

// Source names the catalog origin a backend reports in Status.
const (
	SourceFiles   = "files"
	SourceCluster = "cluster"
)

// newBackendFromObjects is the shared constructor; source distinguishes the
// files and cluster backends in status output while every behavior is shared.
// Command resolvers run under a serializing wrapper so concurrent
// resolveCredential calls never fire multiple Touch ID / desktop prompts at
// once (the gateway resolves every mount before spawn).
func newBackendFromObjects(objects *Objects, runner CommandRunner, source string) (*FilesBackend, error) {
	if objects == nil {
		return nil, errors.New("userk8s: files backend requires loaded objects")
	}
	if runner == nil {
		runner = ExecRunner{}
	}
	return &FilesBackend{objects: objects, runner: &serialRunner{inner: runner}, source: source}, nil
}

// serialRunner runs one resolver command at a time. It is a struct field
// wrapper, not package-level mutable state (one-shot-man-2 standard).
type serialRunner struct {
	inner CommandRunner
	mu    sync.Mutex
}

// Run executes argv with exclusive access to the inner runner.
func (s *serialRunner) Run(ctx context.Context, argv []string, timeout time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Run(ctx, argv, timeout)
}

// ListProviders returns every provider, ordered by name.
func (b *FilesBackend) ListProviders(context.Context) ([]v1alpha1.ModelProvider, error) {
	providers := append([]v1alpha1.ModelProvider(nil), b.objects.Providers...)
	sort.Slice(providers, func(i, j int) bool { return providers[i].Name < providers[j].Name })
	return providers, nil
}

// ListAccesses returns every access, ordered by name. Credential values are
// never included.
func (b *FilesBackend) ListAccesses(context.Context) ([]v1alpha1.ModelAccess, error) {
	accesses := append([]v1alpha1.ModelAccess(nil), b.objects.Accesses...)
	sort.Slice(accesses, func(i, j int) bool { return accesses[i].Name < accesses[j].Name })
	return accesses, nil
}

// ListModels returns every model, ordered by name.
func (b *FilesBackend) ListModels(context.Context) ([]v1alpha1.Model, error) {
	models := append([]v1alpha1.Model(nil), b.objects.Models...)
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })
	return models, nil
}

// ListTools returns every tool, ordered by name.
func (b *FilesBackend) ListTools(context.Context) ([]v1alpha1.Tool, error) {
	tools := append([]v1alpha1.Tool(nil), b.objects.Tools...)
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, nil
}

// Status reports what the backend loaded.
func (b *FilesBackend) Status(context.Context) (BackendStatus, error) {
	source := b.source
	if source == "" {
		source = SourceFiles
	}
	return BackendStatus{
		Source: source,
		// Non-nil so an absent artifact list encodes as [] rather than null;
		// every backend must expose the same shape.
		Artifacts: append([]string{}, b.objects.Artifacts...),
		Providers: len(b.objects.Providers),
		Accesses:  len(b.objects.Accesses),
		Models:    len(b.objects.Models),
		Tools:     len(b.objects.Tools),
		Bindings:  len(b.objects.Bindings),
	}, nil
}

// Access returns one access by name, accepting either the metadata name or the
// raw registry name.
func (b *FilesBackend) Access(name string) (*v1alpha1.ModelAccess, error) {
	access, ok := b.objects.lookupAccess(name)
	if !ok {
		return nil, fmt.Errorf("unknown ModelAccess %q", name)
	}
	return access, nil
}

// Model returns one model by name, accepting either the metadata name or the
// raw registry name.
func (b *FilesBackend) Model(name string) (*v1alpha1.Model, error) {
	model, ok := b.objects.lookupModel(name)
	if !ok {
		return nil, fmt.Errorf("unknown Model %q", name)
	}
	return model, nil
}

// Tool returns one tool by name.
func (b *FilesBackend) Tool(name string) (*v1alpha1.Tool, error) {
	tool, ok := b.objects.toolByName[name]
	if !ok {
		return nil, fmt.Errorf("unknown Tool %q", name)
	}
	return tool, nil
}

// Provider returns one provider by name, accepting either the metadata name or
// the raw registry name.
func (b *FilesBackend) Provider(name string) (*v1alpha1.ModelProvider, error) {
	provider, ok := b.objects.lookupProvider(name)
	if !ok {
		return nil, fmt.Errorf("unknown ModelProvider %q", name)
	}
	return provider, nil
}

// ResolveCredential implements the resolution protocol: bindings whose
// selector matches the access and whose envVar is named by auth.requiredEnv
// are executed in order, first success wins; the access is resolved only when
// every required name is covered.
func (b *FilesBackend) ResolveCredential(ctx context.Context, accessRef string) (Resolution, error) {
	if err := ctx.Err(); err != nil {
		return Resolution{}, err
	}
	access, err := b.Access(accessRef)
	if err != nil {
		return Resolution{}, err
	}
	if access.Spec.Auth == nil {
		return Resolution{}, fmt.Errorf("ModelAccess %q declares no spec.auth", accessRef)
	}

	switch access.Spec.Auth.Scheme {
	case v1alpha1.AuthSchemeNone:
		return Resolution{Status: StatusResolved, Reason: "auth scheme none requires no credentials"}, nil
	case v1alpha1.AuthSchemeBearer:
		// Resolved below.
	case v1alpha1.AuthSchemeAWSSigV4, v1alpha1.AuthSchemeToolManaged:
		return Resolution{
			Status: StatusUnsupportedScheme,
			Reason: fmt.Sprintf("auth scheme %s is modeled but not launchable in this version", access.Spec.Auth.Scheme),
		}, nil
	default:
		return Resolution{
			Status: StatusUnsupportedScheme,
			Reason: fmt.Sprintf("auth scheme %q is not recognized", access.Spec.Auth.Scheme),
		}, nil
	}

	set := labelSet(access)
	resolution := Resolution{Status: StatusResolved}
	var missing []string
	var notes []string
	for _, slot := range access.Spec.Auth.RequiredEnv {
		matched, err := bindingsForSlot(b.objects.Bindings, set, slot)
		if err != nil {
			return Resolution{}, err
		}
		resolved := false
		for _, binding := range matched {
			credential, ok, failures := resolveBinding(ctx, binding, b.runner)
			if ok {
				resolution.Credentials = append(resolution.Credentials, credential)
				resolved = true
				break
			}
			// A failing resolver is a non-fatal miss: the next step or binding
			// may still cover the slot. Only the failure class is retained —
			// never resolver error text, which could embed a credential.
			for _, failure := range failures {
				notes = append(notes, binding.Name+" "+failure)
			}
		}
		if !resolved {
			missing = append(missing, slot)
		}
	}

	if len(missing) > 0 {
		resolution.Status = StatusMissingCredentials
		reason := fmt.Sprintf("no selector-matched LocalSecretBinding resolved %s", joinQuoted(missing))
		if summary := attemptSummary(notes); summary != "" {
			reason += " (attempts: " + summary + ")"
		}
		resolution.Reason = reason
	}
	return resolution, nil
}

// maxAttemptNotes bounds how many resolver misses are reported, so a large
// catalog cannot produce an unbounded message.
const maxAttemptNotes = 8

// attemptSummary renders resolver misses for a human, bounded in both count
// and length. Entries are failure classes such as
// "bind-alpha command op: failed" and never contain resolver output.
func attemptSummary(notes []string) string {
	if len(notes) == 0 {
		return ""
	}
	shown := notes
	truncated := 0
	if len(shown) > maxAttemptNotes {
		truncated = len(shown) - maxAttemptNotes
		shown = shown[:maxAttemptNotes]
	}
	summary := strings.Join(shown, "; ")
	if truncated > 0 {
		summary += fmt.Sprintf("; and %d more", truncated)
	}
	return summary
}

// labelSet is the label set a LocalSecretBinding selector matches against:
// metadata labels merged with the mirroring spec labels.
func labelSet(access *v1alpha1.ModelAccess) labels.Set {
	set := labels.Set{}
	maps.Copy(set, access.Spec.Labels)
	maps.Copy(set, access.Labels)
	return set
}

// joinQuoted renders a sorted, quoted, comma-separated list for messages.
func joinQuoted(values []string) string {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	quoted := make([]string, 0, len(sorted))
	for _, value := range sorted {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return strings.Join(quoted, ", ")
}
