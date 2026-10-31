package userk8s

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

// Catalog is the read-only view of the model catalog every backend
// implements. Files and cluster backends are interchangeable: adapters and the
// scripting surface depend on this interface only.
type Catalog interface {
	ListProviders(ctx context.Context) ([]v1alpha1.ModelProvider, error)
	ListAccesses(ctx context.Context) ([]v1alpha1.ModelAccess, error)
	ListModels(ctx context.Context) ([]v1alpha1.Model, error)
	ListServings(ctx context.Context) ([]v1alpha1.ModelServing, error)
	ListSelections(ctx context.Context) ([]v1alpha1.ModelSelection, error)
	ListRoutes(ctx context.Context) ([]v1alpha1.ModelRoute, error)
	ListTools(ctx context.Context) ([]v1alpha1.Tool, error)
	ListBindings(ctx context.Context) ([]v1alpha1.LocalSecretBinding, error)

	// EffectiveModel applies one serving's narrowings to its Model.
	EffectiveModel(ctx context.Context, servingRef string) (EffectiveModel, error)

	// RouteOffers resolves one route's effective offering inventory.
	RouteOffers(ctx context.Context, routeRef string) ([]ResolvedOffer, error)

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
	return &FilesBackend{objects: objects, runner: newSerialRunner(runner), source: source}, nil
}

// serialRunner runs one resolver command at a time. It is a struct field
// wrapper, not package-level mutable state (one-shot-man-2 standard).
type serialRunner struct {
	inner CommandRunner
	gate  chan struct{}
}

func newSerialRunner(inner CommandRunner) *serialRunner {
	return &serialRunner{inner: inner, gate: make(chan struct{}, 1)}
}

// Run executes argv with exclusive access to the inner runner. The gate wait is
// context-aware, so a caller cancelled while another command is in flight
// returns promptly instead of blocking for that command's whole timeout (which
// can be 60s for a human Touch ID approval).
func (s *serialRunner) Run(ctx context.Context, argv []string, timeout time.Duration) (string, error) {
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	defer func() { <-s.gate }()
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

// ListServings returns every provider offering, ordered by name.
func (b *FilesBackend) ListServings(context.Context) ([]v1alpha1.ModelServing, error) {
	servings := append([]v1alpha1.ModelServing(nil), b.objects.Servings...)
	sort.Slice(servings, func(i, j int) bool { return servings[i].Name < servings[j].Name })
	return servings, nil
}

// ListSelections returns every fallback selection, ordered by name.
func (b *FilesBackend) ListSelections(context.Context) ([]v1alpha1.ModelSelection, error) {
	selections := append([]v1alpha1.ModelSelection(nil), b.objects.Selections...)
	sort.Slice(selections, func(i, j int) bool { return selections[i].Name < selections[j].Name })
	return selections, nil
}

// EffectiveModel resolves one serving to its effective capabilities.
func (b *FilesBackend) EffectiveModel(_ context.Context, servingRef string) (EffectiveModel, error) {
	serving, ok := b.objects.lookupServing(servingRef)
	if !ok {
		return EffectiveModel{}, fmt.Errorf("unknown ModelServing %q", servingRef)
	}
	model, ok := b.objects.lookupModel(serving.Spec.Model)
	if !ok {
		return EffectiveModel{}, fmt.Errorf("ModelServing %q references undeclared Model %q", serving.Name, serving.Spec.Model)
	}
	return effectiveModel(model, serving)
}

// RouteOffers resolves one route's effective offering inventory.
func (b *FilesBackend) RouteOffers(_ context.Context, routeRef string) ([]ResolvedOffer, error) {
	route, ok := b.objects.lookupRoute(routeRef)
	if !ok {
		return nil, fmt.Errorf("unknown ModelRoute %q", routeRef)
	}
	return b.objects.routeOffers(route)
}

// ListBindings returns every credential binding, ordered by name.
func (b *FilesBackend) ListBindings(context.Context) ([]v1alpha1.LocalSecretBinding, error) {
	bindings := append([]v1alpha1.LocalSecretBinding(nil), b.objects.Bindings...)
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].Name < bindings[j].Name })
	return bindings, nil
}

// ListRoutes returns every route, ordered by name.
func (b *FilesBackend) ListRoutes(context.Context) ([]v1alpha1.ModelRoute, error) {
	routes := append([]v1alpha1.ModelRoute(nil), b.objects.Routes...)
	sort.Slice(routes, func(i, j int) bool { return routes[i].Name < routes[j].Name })
	return routes, nil
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
		Routes:    len(b.objects.Routes),
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

// Route returns one route by name, accepting either the metadata name or the
// raw registry name.
func (b *FilesBackend) Route(name string) (*v1alpha1.ModelRoute, error) {
	route, ok := b.objects.lookupRoute(name)
	if !ok {
		return nil, fmt.Errorf("unknown ModelRoute %q", name)
	}
	return route, nil
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

// ResolveCredential resolves one entity-held credential binding. It is a
// binding lookup, not an access lookup: credentials belong to the entity and
// are named by a ModelRoute, never attached to a Model or a ModelAccess.
func (b *FilesBackend) ResolveCredential(ctx context.Context, bindingRef string) (Resolution, error) {
	if err := ctx.Err(); err != nil {
		return Resolution{}, err
	}
	binding, ok := b.objects.lookupBinding(bindingRef)
	if !ok {
		return Resolution{}, fmt.Errorf("unknown LocalSecretBinding %q", bindingRef)
	}
	switch binding.Spec.Scheme {
	case "", v1alpha1.CredentialNone:
		return Resolution{Status: StatusResolved, Reason: "credential scheme none requires no value"}, nil
	case v1alpha1.CredentialBearer, v1alpha1.CredentialAPIKey, v1alpha1.CredentialXAPIKey:
		credential, resolved, failures := resolveBinding(ctx, *binding, b.runner)
		if resolved {
			return Resolution{Status: StatusResolved, Credentials: []Credential{credential}}, nil
		}
		reason := "LocalSecretBinding " + binding.Name + " did not resolve"
		if summary := attemptSummary(failures); summary != "" {
			reason += " (attempts: " + summary + ")"
		}
		return Resolution{Status: StatusMissingCredentials, Reason: reason}, nil
	case v1alpha1.CredentialAWSSigV4:
		return Resolution{
			Status: StatusUnsupportedScheme,
			Reason: fmt.Sprintf("credential scheme %s is modeled but not launchable in this version", binding.Spec.Scheme),
		}, nil
	default:
		return Resolution{
			Status: StatusUnsupportedScheme,
			Reason: fmt.Sprintf("credential scheme %q is not recognized", binding.Spec.Scheme),
		}, nil
	}
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
