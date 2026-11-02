// Package userk8smod provides the Goja module registered as "osm:userk8s":
// the typed catalog API over the userk8s engine. It exposes the model
// catalog the launcher and tool adapters read, resolves an access's
// credentials through its selector-matched LocalSecretBindings, and projects a
// (tool, provider, model) selection into the slug pair and settings a tool
// adapter launches with.
//
// The async contract: load() returns Promise<Snapshot>, resolveCredential
// (bindingRef, signal?) returns Promise<Resolution> honoring an optional
// AbortSignal, and project(tool, provider, serving) and backendStatus() return
// promises because their first call may load the catalog. Rejections carry
// typed .code values (catalog-not-found, binding-not-found,
// credential-unresolved, resolver-failure-class, and the ABORT_ERR abort);
// resolver failures surface only as their class, never raw error text, and
// credential values cross into JS only as the value field of a resolved
// credential.
//
// Credential values cross into JS only as the value field of a resolved
// credential; nothing else in the surface exposes credential material, and
// resolver failures are reported as classes rather than error text.
package userk8smod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/one-shot-man/internal/userk8s"
	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

// SourceFiles reads the catalog from rendered profile artifacts. SourceCluster
// reads it from a Kubernetes API server; it is wired by the cluster backend.
const (
	SourceFiles   = "files"
	SourceCluster = "cluster"
)

// Options is the resolved userk8s configuration the module runs with.
type Options struct {
	// Source selects the backend: SourceFiles (default) or SourceCluster.
	Source string
	// Artifacts are the rendered profile files or directories the files
	// backend loads.
	Artifacts []string
	// Namespace scopes namespaced kinds (LocalSecretBinding, Secret).
	Namespace string
	// Kubeconfig and Context configure the cluster backend, defaulting to the
	// standard resolution rules when empty.
	Kubeconfig string
	Context    string
	// Runner executes command resolvers; nil uses the production exec runner.
	Runner userk8s.CommandRunner
}

// Provider builds a Catalog for the configured source. The cluster backend
// supplies its own so this package never imports client-go.
type Provider func(ctx context.Context, options Options) (userk8s.Catalog, error)

// filesProvider implements the only source this package builds itself. A
// request for any other source is refused rather than silently served from
// files, so a misconfiguration cannot masquerade as a working cluster backend.
func filesProvider(_ context.Context, options Options) (userk8s.Catalog, error) {
	switch options.Source {
	case "", SourceFiles:
	case SourceCluster:
		return nil, fmt.Errorf("osm:userk8s: userk8s.source %q requires the cluster backend, which this build does not provide", options.Source)
	default:
		return nil, fmt.Errorf("osm:userk8s: userk8s.source %q is not one of %s, %s", options.Source, SourceFiles, SourceCluster)
	}
	if len(options.Artifacts) == 0 {
		return nil, fmt.Errorf("osm:userk8s: userk8s.artifacts must name at least one rendered profile artifact")
	}
	return userk8s.NewFilesBackend(userk8s.FilesBackendOptions{Paths: options.Artifacts, Runner: options.Runner})
}

// stringArg reads one string argument, rejecting an omitted, null, or blank
// value instead of stringifying it into the literal "undefined".
func stringArg(call goja.FunctionCall, index int) (string, bool) {
	if index >= len(call.Arguments) {
		return "", false
	}
	value := call.Arguments[index]
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return "", false
	}
	trimmed := strings.TrimSpace(value.String())
	return trimmed, trimmed != ""
}

// clusterProvider builds the cluster backend, which reads the same catalog
// kinds from a Kubernetes API server and reuses the shared catalog logic.
func clusterProvider(_ context.Context, options Options) (userk8s.Catalog, error) {
	return userk8s.NewClusterBackend(userk8s.ClusterBackendOptions{
		Namespace:  options.Namespace,
		Kubeconfig: options.Kubeconfig,
		Context:    options.Context,
		Runner:     options.Runner,
	})
}

// Require returns the Goja module loader for osm:userk8s, dispatching on the
// configured source so a source change flips the backend without any
// JavaScript-visible difference. adapter supplies the async machinery
// (TrackPromise / TrackAbortSignal / NewPromise); the async surface panics at
// call time when it is nil.
func Require(ctx context.Context, options Options, adapter *gojaeventloop.Adapter) func(runtime *goja.Runtime, module *goja.Object) {
	build := filesProvider
	if options.Source == SourceCluster {
		build = clusterProvider
	}
	return RequireWithProvider(ctx, options, build, adapter)
}

// RequireWithProvider is Require with an injectable backend, so the cluster
// backend can be wired without this package importing client-go.
//
// The async contract: all catalog operations return promises because the
// first call can load files or query a cluster. Blocking work settles through
// Adapter.TrackPromise, and an optional AbortSignal aborts resolveCredential
// with the standard AbortError.
// Rejections carry typed .code values: catalog-not-found, binding-not-found,
// credential-unresolved, and resolver-failure-class — and resolver failures are
// always reported as classes, never raw error text.
func RequireWithProvider(ctx context.Context, options Options, build Provider, adapter *gojaeventloop.Adapter) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)
		lazy := &lazyCatalog{options: options, build: build}

		// load() -> Promise<Snapshot>: {source, providers, models, accesses,
		// tools, secretsPresent}
		_ = exports.Set("load", func(call goja.FunctionCall) goja.Value {
			if adapter == nil {
				panic(runtime.NewTypeError("osm:userk8s: load requires the async runtime wiring"))
			}
			return adapter.TrackPromise(ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				catalog, err := lazy.get(ctx)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return catalogError(rt, "catalog-not-found", err) })
					return
				}
				loaded, err := loadCatalog(ctx, catalog)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return catalogError(rt, "catalog-not-found", err) })
					return
				}
				_ = settle.Settle(false, func(rt *goja.Runtime) any { return toJS(rt, loaded) })
			})
		})

		// resolveCredential(bindingRef, signal?) -> Promise<Resolution>
		_ = exports.Set("resolveCredential", func(call goja.FunctionCall) goja.Value {
			if adapter == nil {
				panic(runtime.NewTypeError("osm:userk8s: resolveCredential requires the async runtime wiring"))
			}
			bindingRef, ok := stringArg(call, 0)
			if !ok {
				promise, settler := adapter.NewPromise()
				_ = settler.Reject(func(rt *goja.Runtime) any {
					return rt.NewTypeError("osm:userk8s: resolveCredential requires a LocalSecretBinding reference")
				})
				return promise
			}
			signalVal := signalArg(call, 1)

			reqCtx, cancel := context.WithCancel(ctx)
			var abortCleanup func()
			if signalVal != nil {
				if cleanup, aborted, ok := adapter.TrackAbortSignal(signalVal, func() { cancel() }); ok {
					abortCleanup = cleanup
					if aborted {
						cancel()
						return abortedPromise(adapter, signalVal)
					}
				}
			}

			return adapter.TrackPromise(reqCtx, func(trackCtx context.Context, settle gojaeventloop.TrackedSettlement) {
				defer cancel()
				if abortCleanup != nil {
					defer abortCleanup()
				}
				catalog, err := lazy.get(trackCtx)
				if err != nil {
					if trackCtx.Err() != nil {
						_ = settle.Settle(true, func(rt *goja.Runtime) any { return abortError(rt) })
						return
					}
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return catalogError(rt, "catalog-not-found", err) })
					return
				}
				if err := trackCtx.Err(); err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return abortError(rt) })
					return
				}
				resolution, err := catalog.ResolveCredential(trackCtx, bindingRef)
				if err != nil {
					// An abort that landed mid-resolution wins over every
					// classification: the resolver's context cancellation IS
					// the abort.
					if trackCtx.Err() != nil {
						_ = settle.Settle(true, func(rt *goja.Runtime) any { return abortError(rt) })
						return
					}
					// An unknown access reference is its own typed code; any
					// other error is a resolver failure reported as the class
					// alone — never raw resolver output.
					code := "resolver-failure-class"
					if strings.Contains(err.Error(), "unknown LocalSecretBinding") {
						code = "binding-not-found"
					}
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return catalogError(rt, code, err) })
					return
				}
				if resolution.Status == userk8s.StatusMissingCredentials {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return credentialUnresolved(rt, resolution) })
					return
				}
				_ = settle.Settle(false, func(rt *goja.Runtime) any { return toJS(rt, resolution) })
			})
		})

		// project(toolName, providerName, servingName) -> Promise<Projection>
		_ = exports.Set("project", func(call goja.FunctionCall) goja.Value {
			if adapter == nil {
				panic(runtime.NewTypeError("osm:userk8s: project requires the async runtime wiring"))
			}
			toolName, toolOK := stringArg(call, 0)
			providerName, providerOK := stringArg(call, 1)
			servingName, servingOK := stringArg(call, 2)
			if !toolOK || !providerOK || !servingOK {
				return adapter.TrackPromise(ctx, func(_ context.Context, settle gojaeventloop.TrackedSettlement) {
					_ = settle.Settle(true, func(rt *goja.Runtime) any {
						return rt.NewTypeError("osm:userk8s: project requires a tool, a provider, and a serving")
					})
				})
			}
			return adapter.TrackPromise(ctx, func(trackCtx context.Context, settle gojaeventloop.TrackedSettlement) {
				catalog, err := lazy.get(trackCtx)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return catalogError(rt, "catalog-not-found", err) })
					return
				}
				projection, err := catalog.Project(trackCtx, toolName, providerName, servingName)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any {
						return rt.NewTypeError(err.Error())
					})
					return
				}
				_ = settle.Settle(false, func(rt *goja.Runtime) any { return toJS(rt, projection) })
			})
		})

		// backendStatus() -> Promise<Status>
		_ = exports.Set("backendStatus", func(call goja.FunctionCall) goja.Value {
			if adapter == nil {
				panic(runtime.NewTypeError("osm:userk8s: backendStatus requires the async runtime wiring"))
			}
			return adapter.TrackPromise(ctx, func(trackCtx context.Context, settle gojaeventloop.TrackedSettlement) {
				catalog, err := lazy.get(trackCtx)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return catalogError(rt, "catalog-not-found", err) })
					return
				}
				status, err := catalog.Status(trackCtx)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return catalogError(rt, "catalog-not-found", err) })
					return
				}
				_ = settle.Settle(false, func(rt *goja.Runtime) any { return toJS(rt, status) })
			})
		})
	}
}

// errResolverClass is the only text a resolver failure surfaces: the class
// name, with no resolver output, exit status, or environment detail.
var errResolverClass = errors.New("credential resolver failed")

// catalogError builds a rejected reason with a typed code. The
// resolver-failure-class code never carries the underlying error: resolver
// stdout, stderr, exit status, and environment detail stay in Go. The other
// codes (catalog-not-found, binding-not-found) keep the underlying message —
// those errors name catalog objects and configuration, not resolver output.
func catalogError(rt *goja.Runtime, code string, err error) *goja.Object {
	if code == "resolver-failure-class" {
		err = errResolverClass
	}
	obj := rt.NewGoError(errors.New(code + ": " + err.Error()))
	_ = obj.Set("code", code)
	return obj
}

// credentialUnresolved rejects with the typed missing-credentials code while
// preserving the documented resolution fields (status, reason) — but never
// credential values, because there are none.
func credentialUnresolved(rt *goja.Runtime, resolution userk8s.Resolution) *goja.Object {
	reason := resolution.Reason
	if reason == "" {
		reason = "required credentials are missing"
	}
	obj := rt.NewGoError(errors.New("credential-unresolved: " + reason))
	_ = obj.Set("code", "credential-unresolved")
	_ = obj.Set("status", string(resolution.Status))
	_ = obj.Set("reason", reason)
	return obj
}

// abortError builds the standard AbortError DOMException-shaped rejection.
func abortError(rt *goja.Runtime) *goja.Object {
	obj := rt.NewGoError(errors.New("this operation was aborted"))
	_ = obj.Set("code", "ABORT_ERR")
	_ = obj.Set("name", "AbortError")
	return obj
}

// abortedPromise settles immediately with the signal's own reason (Node
// semantics: reject with signal.reason when present, AbortError otherwise).
func abortedPromise(adapter *gojaeventloop.Adapter, signalVal goja.Value) goja.Value {
	promise, settler := adapter.NewPromise()
	_ = settler.Reject(func(rt *goja.Runtime) any {
		if sigObj, ok := signalVal.(*goja.Object); ok {
			if reason := sigObj.Get("reason"); reason != nil && !goja.IsUndefined(reason) && !goja.IsNull(reason) {
				return reason
			}
		}
		return abortError(rt)
	})
	return promise
}

// signalArg reads an optional AbortSignal argument.
func signalArg(call goja.FunctionCall, index int) goja.Value {
	if index >= len(call.Arguments) {
		return nil
	}
	value := call.Arguments[index]
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil
	}
	return value
}

// toJS converts a typed payload into a plain JavaScript object that honors the
// payload's json tags, so scripts see the documented lowercase keys rather
// than Go field names.
func toJS(runtime *goja.Runtime, value any) goja.Value {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(runtime.NewTypeError(fmt.Sprintf("osm:userk8s: encoding a result failed: %v", err)))
	}
	var generic any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		panic(runtime.NewTypeError(fmt.Sprintf("osm:userk8s: decoding a result failed: %v", err)))
	}
	return runtime.ToValue(generic)
}

// lazyCatalog builds the backend on first use and reuses it afterwards, so a
// configuration or filesystem error surfaces at the call site rather than
// aborting module registration. get() is safe for concurrent callers and
// always runs inside a tracked-promise goroutine.
type lazyCatalog struct {
	options Options
	build   Provider

	mu      sync.Mutex
	catalog userk8s.Catalog
	err     error
}

func (l *lazyCatalog) get(ctx context.Context) (userk8s.Catalog, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.catalog != nil || l.err != nil {
		return l.catalog, l.err
	}
	if err := validateSource(l.options.Source); err != nil {
		l.err = err
		return nil, l.err
	}
	catalog, err := l.build(ctx, l.options)
	if err != nil && ctx.Err() != nil {
		return nil, err
	}
	l.catalog, l.err = catalog, err
	return l.catalog, l.err
}

// validateSource rejects a source this module cannot serve, so an
// environment-provided value cannot bypass the schema's enum (the schema
// validates the configuration file, while this guard covers every caller).
func validateSource(source string) error {
	switch source {
	case "", SourceFiles, SourceCluster:
		return nil
	default:
		return fmt.Errorf("osm:userk8s: userk8s.source %q is not one of %s, %s", source, SourceFiles, SourceCluster)
	}
}

// LoadedProviders is the JS-facing shape of a ModelProvider.
type LoadedProviders struct {
	Name        string `json:"name"`
	Registry    string `json:"registryName"`
	DisplayName string `json:"displayName"`
	Country     string `json:"country,omitempty"`
}

// LoadedAccess is the JS-facing shape of a ModelAccess. It carries only the
// logical provider/protocol channel; credentials and transports live on entity
// bindings and routes.
type LoadedAccess struct {
	Name        string            `json:"name"`
	Registry    string            `json:"registryName"`
	Provider    string            `json:"provider"`
	Protocol    string            `json:"protocol"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Deprecated  bool              `json:"deprecated,omitempty"`
}

// LoadedShaper is the JS-facing shape of ShaperTransportConfig.
type LoadedShaper struct {
	AuthMode        string `json:"authMode,omitempty"`
	Concurrency     *int32 `json:"concurrency,omitempty"`
	LimitAll        *bool  `json:"limitAll,omitempty"`
	QueueTimeout    string `json:"queueTimeout,omitempty"`
	Retry           *int32 `json:"retry,omitempty"`
	RetryMinDelay   string `json:"retryMinDelay,omitempty"`
	RetrySkip429    *bool  `json:"retrySkip429,omitempty"`
	ReleaseCooldown string `json:"releaseCooldown,omitempty"`
	CancelCooldown  string `json:"cancelCooldown,omitempty"`
	CircuitBreaker  *bool  `json:"circuitBreaker,omitempty"`
	CBThreshold     *int32 `json:"cbThreshold,omitempty"`
	CBWindow        string `json:"cbWindow,omitempty"`
	CBPenalty       string `json:"cbPenalty,omitempty"`
	CBMaxPenalty    string `json:"cbMaxPenalty,omitempty"`
}

// LoadedModel is the JS-facing shape of a provider-independent Model.
type LoadedModel struct {
	Name             string                   `json:"name"`
	Registry         string                   `json:"registryName"`
	CanonicalID      string                   `json:"canonicalId"`
	Family           string                   `json:"family,omitempty"`
	DisplayName      string                   `json:"displayName"`
	Aliases          []string                 `json:"aliases,omitempty"`
	ContextWindow    int64                    `json:"contextWindow"`
	MaxOutputTokens  int64                    `json:"maxOutputTokens"`
	InputModalities  []string                 `json:"inputModalities,omitempty"`
	OutputModalities []string                 `json:"outputModalities,omitempty"`
	Reasoning        *v1alpha1.ModelReasoning `json:"reasoning,omitempty"`
	ToolCall         bool                     `json:"toolCall"`
	StructuredOutput bool                     `json:"structuredOutput"`
	Deprecated       bool                     `json:"deprecated,omitempty"`
}

// LoadedEffective is the effective capability set of one serving (Model plus
// its provider narrowing overrides). Consumers must not merge the two
// themselves.
type LoadedEffective struct {
	ContextWindow    int64                    `json:"contextWindow"`
	MaxOutputTokens  int64                    `json:"maxOutputTokens"`
	InputModalities  []string                 `json:"inputModalities,omitempty"`
	OutputModalities []string                 `json:"outputModalities,omitempty"`
	Reasoning        *v1alpha1.ModelReasoning `json:"reasoning,omitempty"`
	ToolCall         bool                     `json:"toolCall"`
	StructuredOutput bool                     `json:"structuredOutput"`
}

// LoadedOverrides is the JS-facing shape of one serving's capability
// narrowings. It mirrors the CRD struct but uses camelCase keys, so the served
// payload has one vocabulary: the snake_case CRD spelling would sit beside
// camelCase effective values and force every script to know both.
type LoadedOverrides struct {
	ContextWindow    *int64                   `json:"contextWindow,omitempty"`
	MaxOutputTokens  *int64                   `json:"maxOutputTokens,omitempty"`
	InputModalities  []string                 `json:"inputModalities,omitempty"`
	OutputModalities []string                 `json:"outputModalities,omitempty"`
	Reasoning        *v1alpha1.ModelReasoning `json:"reasoning,omitempty"`
	ToolCall         *bool                    `json:"toolCall,omitempty"`
	StructuredOutput *bool                    `json:"structuredOutput,omitempty"`
}

// LoadedServing is the JS-facing shape of one provider offering.
type LoadedServing struct {
	Name            string           `json:"name"`
	Registry        string           `json:"registryName"`
	Provider        string           `json:"provider"`
	Model           string           `json:"model"`
	ProviderModelID string           `json:"providerModelId"`
	Priority        int32            `json:"priority"`
	Default         bool             `json:"default,omitempty"`
	Deprecated      bool             `json:"deprecated,omitempty"`
	DisplayName     string           `json:"displayName"`
	Effective       LoadedEffective  `json:"effective"`
	Overrides       *LoadedOverrides `json:"overrides,omitempty"`
}

// LoadedBinding is the JS-facing shape of a credential binding. It carries no
// value, only the slot name and scheme a route needs to resolve it.
type LoadedBinding struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Scheme      string            `json:"scheme,omitempty"`
	EnvVar      string            `json:"envVar"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// LoadedRouteOffer is one served id a route exposes.
type LoadedRouteOffer struct {
	ID      string `json:"id"`
	Serving string `json:"serving"`
}

// LoadedRoute is the JS-facing shape of one entity access item.
type LoadedRoute struct {
	Name        string             `json:"name"`
	Registry    string             `json:"registryName"`
	Namespace   string             `json:"namespace,omitempty"`
	Access      string             `json:"access,omitempty"`
	Credential  string             `json:"credential,omitempty"`
	Offers      []LoadedRouteOffer `json:"offers"`
	Surfaces    []string           `json:"surfaces"`
	Endpoints   map[string]string  `json:"endpoints"`
	Naming      string             `json:"naming"`
	Order       int32              `json:"order"`
	Transport   LoadedTransport    `json:"transport"`
	Annotations map[string]string  `json:"annotations,omitempty"`
}

// LoadedTransport is the JS-facing shape of ModelRouteTransport.
type LoadedTransport struct {
	Kind     string        `json:"kind"`
	Upstream string        `json:"upstream,omitempty"`
	Shaper   *LoadedShaper `json:"shaper,omitempty"`
}

// LoadedSelection is the JS-facing shape of one fallback selection.
type LoadedSelection struct {
	Name        string                             `json:"name"`
	Registry    string                             `json:"registryName"`
	Namespace   string                             `json:"namespace,omitempty"`
	DisplayName string                             `json:"displayName"`
	Strategy    string                             `json:"strategy"`
	Candidates  []v1alpha1.ModelSelectionCandidate `json:"candidates"`
}

// LoadedTool is the JS-facing shape of a Tool.
type LoadedTool struct {
	Name               string   `json:"name"`
	DisplayName        string   `json:"displayName"`
	Surfaces           []string `json:"surfaces"`
	CredentialChannels []string `json:"credentialChannels"`
	BudgetProfile      string   `json:"budgetProfile"`
}

// Loaded is the payload of load().
type Loaded struct {
	Source         string            `json:"source"`
	Providers      []LoadedProviders `json:"providers"`
	Models         []LoadedModel     `json:"models"`
	Servings       []LoadedServing   `json:"servings"`
	Accesses       []LoadedAccess    `json:"accesses"`
	Bindings       []LoadedBinding   `json:"bindings"`
	Selections     []LoadedSelection `json:"selections"`
	Routes         []LoadedRoute     `json:"routes"`
	Tools          []LoadedTool      `json:"tools"`
	SecretsPresent int               `json:"secretsPresent"`
}

// loadCatalog reads every kind through the Catalog interface, so the payload
// is identical for the files and cluster backends.
func loadCatalog(ctx context.Context, catalog userk8s.Catalog) (Loaded, error) {
	status, err := catalog.Status(ctx)
	if err != nil {
		return Loaded{}, err
	}
	providers, err := catalog.ListProviders(ctx)
	if err != nil {
		return Loaded{}, err
	}
	accesses, err := catalog.ListAccesses(ctx)
	if err != nil {
		return Loaded{}, err
	}
	models, err := catalog.ListModels(ctx)
	if err != nil {
		return Loaded{}, err
	}
	servings, err := catalog.ListServings(ctx)
	if err != nil {
		return Loaded{}, err
	}
	selections, err := catalog.ListSelections(ctx)
	if err != nil {
		return Loaded{}, err
	}
	routes, err := catalog.ListRoutes(ctx)
	if err != nil {
		return Loaded{}, err
	}
	tools, err := catalog.ListTools(ctx)
	if err != nil {
		return Loaded{}, err
	}
	bindings, err := catalog.ListBindings(ctx)
	if err != nil {
		return Loaded{}, err
	}

	loaded := Loaded{
		Source:         status.Source,
		Providers:      make([]LoadedProviders, 0, len(providers)),
		Models:         make([]LoadedModel, 0, len(models)),
		Servings:       make([]LoadedServing, 0, len(servings)),
		Accesses:       make([]LoadedAccess, 0, len(accesses)),
		Bindings:       make([]LoadedBinding, 0, len(bindings)),
		Selections:     make([]LoadedSelection, 0, len(selections)),
		Routes:         make([]LoadedRoute, 0, len(routes)),
		Tools:          make([]LoadedTool, 0, len(tools)),
		SecretsPresent: status.Bindings,
	}
	for i := range providers {
		loaded.Providers = append(loaded.Providers, LoadedProviders{
			Name:        providers[i].Name,
			Registry:    registryName(providers[i].Name, providers[i].Annotations),
			DisplayName: providers[i].Spec.DisplayName,
			Country:     providers[i].Spec.Country,
		})
	}
	for i := range accesses {
		access := &accesses[i]
		view := LoadedAccess{
			Name:        access.Name,
			Registry:    registryName(access.Name, access.Annotations),
			Provider:    access.Spec.Provider,
			Protocol:    string(access.Spec.Protocol),
			Labels:      map[string]string{},
			Annotations: map[string]string{},
			Deprecated:  access.Spec.Deprecated,
		}
		maps.Copy(view.Labels, access.Labels)
		maps.Copy(view.Annotations, access.Annotations)
		loaded.Accesses = append(loaded.Accesses, view)
	}
	// Index by both the metadata name and the raw registry name, with registry
	// precedence matching the engine's lookup: serving.Spec.Model is authored in
	// registry vocabulary, so a metadata-name-only index silently misses the
	// model's display name and deprecation flag. Registry names are written
	// second, so on the collision the engine resolves the same way they win here
	// too.
	modelByName := make(map[string]*v1alpha1.Model, len(models)*2)
	for i := range models {
		modelByName[models[i].Name] = &models[i]
	}
	for i := range models {
		model := &models[i]
		if registry := registryName(model.Name, model.Annotations); registry != model.Name {
			modelByName[registry] = model
		}
	}
	for i := range models {
		model := &models[i]
		loaded.Models = append(loaded.Models, LoadedModel{
			Name:             model.Name,
			Registry:         registryName(model.Name, model.Annotations),
			CanonicalID:      model.Spec.CanonicalID,
			Family:           model.Spec.Family,
			DisplayName:      model.Spec.DisplayName,
			Aliases:          append([]string(nil), model.Spec.Aliases...),
			ContextWindow:    model.Spec.ContextWindow,
			MaxOutputTokens:  model.Spec.MaxOutputTokens,
			InputModalities:  append([]string(nil), model.Spec.InputModalities...),
			OutputModalities: append([]string(nil), model.Spec.OutputModalities...),
			Reasoning:        model.Spec.Reasoning,
			ToolCall:         model.Spec.ToolCall,
			StructuredOutput: model.Spec.StructuredOutput,
			Deprecated:       model.Spec.Deprecated,
		})
	}
	for i := range servings {
		serving := &servings[i]
		// Resolve through the object, not through a name: a registry name and a
		// metadata name may collide, and re-resolving a metadata name that
		// another object also uses as its registry name could pick the wrong
		// model.
		effective, err := effectiveModelFor(catalog, ctx, serving)
		if err != nil {
			return Loaded{}, err
		}
		model := modelByName[serving.Spec.Model]
		display := serving.Name
		modelDeprecated := false
		if model != nil {
			display = model.Spec.DisplayName
			modelDeprecated = model.Spec.Deprecated
		}
		loaded.Servings = append(loaded.Servings, LoadedServing{
			Name:            serving.Name,
			Registry:        registryName(serving.Name, serving.Annotations),
			Provider:        serving.Spec.Provider,
			Model:           serving.Spec.Model,
			ProviderModelID: serving.Spec.ProviderModelID,
			Priority:        serving.Spec.EffectivePriority(),
			Default:         serving.Spec.Default,
			Deprecated:      serving.Spec.Deprecated || modelDeprecated,
			DisplayName:     display,
			Effective: LoadedEffective{
				ContextWindow:    effective.ContextWindow,
				MaxOutputTokens:  effective.MaxOutputTokens,
				InputModalities:  append([]string(nil), effective.InputModalities...),
				OutputModalities: append([]string(nil), effective.OutputModalities...),
				Reasoning:        effective.Reasoning,
				ToolCall:         effective.ToolCall,
				StructuredOutput: effective.StructuredOutput,
			},
			Overrides: loadOverrides(serving.Spec.Overrides),
		})
	}
	for i := range bindings {
		binding := &bindings[i]
		view := LoadedBinding{
			Name:        binding.Name,
			Namespace:   binding.Namespace,
			Scheme:      string(binding.Spec.EffectiveScheme()),
			EnvVar:      binding.Spec.EnvVar,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		}
		maps.Copy(view.Labels, binding.Labels)
		maps.Copy(view.Annotations, binding.Annotations)
		loaded.Bindings = append(loaded.Bindings, view)
	}
	for i := range selections {
		selection := &selections[i]
		view := LoadedSelection{
			Name:        selection.Name,
			Registry:    registryName(selection.Name, selection.Annotations),
			Namespace:   selection.Namespace,
			DisplayName: selection.Spec.DisplayName,
			Strategy:    string(selection.Spec.Strategy),
			Candidates:  append([]v1alpha1.ModelSelectionCandidate(nil), selection.Spec.Candidates...),
		}
		if view.Strategy == "" {
			view.Strategy = string(v1alpha1.SelectionOrdered)
		}
		loaded.Selections = append(loaded.Selections, view)
	}
	for i := range routes {
		route := &routes[i]
		offers, err := routeOffersFor(catalog, ctx, route)
		if err != nil {
			return Loaded{}, err
		}
		view := LoadedRoute{
			Name:       route.Name,
			Registry:   registryName(route.Name, route.Annotations),
			Namespace:  route.Namespace,
			Access:     route.Spec.Access,
			Credential: route.Spec.Credential,
			Offers:     make([]LoadedRouteOffer, 0, len(offers)),
			Surfaces:   append([]string(nil), route.Spec.Surfaces...),
			Endpoints:  routeEndpoints(route),
			Naming:     string(route.Spec.Naming),
			Order:      route.Spec.EffectiveOrder(),
			Transport: LoadedTransport{
				Kind:     string(route.Spec.Transport.Kind),
				Upstream: route.Spec.Transport.Upstream,
				Shaper:   loadShaper(route.Spec.Transport.Shaper),
			},
			Annotations: map[string]string{},
		}
		maps.Copy(view.Annotations, route.Annotations)
		for _, offer := range offers {
			view.Offers = append(view.Offers, LoadedRouteOffer{ID: offer.ID, Serving: offer.Serving.Name})
		}
		loaded.Routes = append(loaded.Routes, view)
	}
	for i := range tools {
		tool := &tools[i]
		loaded.Tools = append(loaded.Tools, LoadedTool{
			Name:               tool.Name,
			DisplayName:        tool.Spec.DisplayName,
			Surfaces:           append([]string(nil), tool.Spec.Surfaces...),
			CredentialChannels: append([]string(nil), tool.Spec.CredentialChannels...),
			BudgetProfile:      tool.Spec.BudgetProfile,
		})
	}
	return loaded, nil
}

// loadOverrides maps a serving's narrowings onto the camelCase JS shape.
func loadOverrides(overrides *v1alpha1.ModelServingOverrides) *LoadedOverrides {
	if overrides == nil {
		return nil
	}
	return &LoadedOverrides{
		ContextWindow:    overrides.ContextWindow,
		MaxOutputTokens:  overrides.MaxOutputTokens,
		InputModalities:  append([]string(nil), overrides.InputModalities...),
		OutputModalities: append([]string(nil), overrides.OutputModalities...),
		Reasoning:        overrides.Reasoning,
		ToolCall:         overrides.ToolCall,
		StructuredOutput: overrides.StructuredOutput,
	}
}

// effectiveModelFor resolves a serving's effective capabilities from the object
// the caller already holds. The Catalog interface accepts a reference because an
// external caller has only a name; this module iterates the objects themselves,
// so it uses the identity it already has rather than a name that may also be
// another object's registry name.
func effectiveModelFor(catalog userk8s.Catalog, ctx context.Context, serving *v1alpha1.ModelServing) (userk8s.EffectiveModel, error) {
	if resolver, ok := catalog.(interface {
		EffectiveModelForServing(context.Context, *v1alpha1.ModelServing) (userk8s.EffectiveModel, error)
	}); ok {
		return resolver.EffectiveModelForServing(ctx, serving)
	}
	return catalog.EffectiveModel(ctx, serving.Name)
}

// routeOffersFor resolves a route's offers from the object the caller holds,
// for the same reason as effectiveModelFor.
func routeOffersFor(catalog userk8s.Catalog, ctx context.Context, route *v1alpha1.ModelRoute) ([]userk8s.ResolvedOffer, error) {
	if resolver, ok := catalog.(interface {
		RouteOffersForRoute(context.Context, *v1alpha1.ModelRoute) ([]userk8s.ResolvedOffer, error)
	}); ok {
		return resolver.RouteOffersForRoute(ctx, route)
	}
	return catalog.RouteOffers(ctx, route.Name)
}

// registryName returns the raw registry identifier an object declares, else
// its metadata name.
func registryName(name string, annotations map[string]string) string {
	if registry := annotations[v1alpha1.RegistryNameAnnotation]; registry != "" {
		return registry
	}
	return name
}

// routeEndpoints copies the route's client bases into the JS-facing map.
func routeEndpoints(route *v1alpha1.ModelRoute) map[string]string {
	endpoints := make(map[string]string, len(route.Spec.Endpoints))
	for surface, endpoint := range route.Spec.Endpoints {
		endpoints[surface] = string(endpoint)
	}
	return endpoints
}

// loadShaper maps a shaper transport's typed tuning onto the JS-facing shape.
func loadShaper(shaper *v1alpha1.ShaperTransportConfig) *LoadedShaper {
	if shaper == nil {
		return nil
	}
	return &LoadedShaper{
		AuthMode:        shaper.AuthMode,
		Concurrency:     shaper.Concurrency,
		LimitAll:        shaper.LimitAll,
		QueueTimeout:    shaper.QueueTimeout,
		Retry:           shaper.Retry,
		RetryMinDelay:   shaper.RetryMinDelay,
		RetrySkip429:    shaper.RetrySkip429,
		ReleaseCooldown: shaper.ReleaseCooldown,
		CancelCooldown:  shaper.CancelCooldown,
		CircuitBreaker:  shaper.CircuitBreaker,
		CBThreshold:     shaper.CBThreshold,
		CBWindow:        shaper.CBWindow,
		CBPenalty:       shaper.CBPenalty,
		CBMaxPenalty:    shaper.CBMaxPenalty,
	}
}
