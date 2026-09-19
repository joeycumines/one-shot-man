// Package userk8smod provides the Goja module registered as "osm:userk8s":
// the typed catalog API over the userk8s engine. It exposes the model
// catalog the launcher and tool adapters read, resolves an access's
// credentials through its selector-matched LocalSecretBindings, and projects a
// (tool, provider, model) selection into the slug pair and settings a tool
// adapter launches with.
//
// The async contract: load() returns Promise<Snapshot>, resolveCredential
// (accessRef, signal?) returns Promise<Resolution> honoring an optional
// AbortSignal, and project(tool, provider, model) stays a pure synchronous
// projection over loaded data. Rejections carry typed .code values
// (catalog-not-found, credential-unresolved, resolver-failure-class, and the
// ABORT_ERR abort); resolver failures surface only as their class, never raw
// error text, and credential values cross into JS only as the value field of
// a resolved credential.
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
// (TrackPromise / TrackAbortSignal / NewPromise); the async surface
// (load, resolveCredential) panics at call time when it is nil.
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
// The async contract: load() and resolveCredential() return promises (their
// I/O settles through Adapter.TrackPromise; load's files-backend reads and
// resolveCredential's resolver execution are the blocking parts), project()
// stays a pure synchronous projection over the loaded state, and an optional
// AbortSignal aborts resolveCredential with the standard AbortError.
// Rejections carry typed .code values: catalog-not-found,
// access-not-found, credential-unresolved, resolver-failure-class — and
// resolver failures are always reported as classes, never raw error text.
func RequireWithProvider(ctx context.Context, options Options, build Provider, adapter *gojaeventloop.Adapter) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)
		lazy := &lazyCatalog{ctx: ctx, options: options, build: build}

		// load() -> Promise<Snapshot>: {source, providers, models, accesses,
		// tools, secretsPresent}
		_ = exports.Set("load", func(call goja.FunctionCall) goja.Value {
			if adapter == nil {
				panic(runtime.NewTypeError("osm:userk8s: load requires the async runtime wiring"))
			}
			return adapter.TrackPromise(ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				catalog, err := lazy.get()
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

		// resolveCredential(accessRef, signal?) -> Promise<Resolution>
		_ = exports.Set("resolveCredential", func(call goja.FunctionCall) goja.Value {
			if adapter == nil {
				panic(runtime.NewTypeError("osm:userk8s: resolveCredential requires the async runtime wiring"))
			}
			accessRef, ok := stringArg(call, 0)
			if !ok {
				promise, settler := adapter.NewPromise()
				_ = settler.Reject(func(rt *goja.Runtime) any {
					return rt.NewTypeError("osm:userk8s: resolveCredential requires an access reference")
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
				catalog, err := lazy.get()
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return catalogError(rt, "catalog-not-found", err) })
					return
				}
				if err := trackCtx.Err(); err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return abortError(rt) })
					return
				}
				resolution, err := catalog.ResolveCredential(trackCtx, accessRef)
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
					if strings.Contains(err.Error(), "unknown ModelAccess") {
						code = "access-not-found"
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

		// project(toolName, providerName, modelName): pure synchronous
		// projection over the loaded catalog: {providerSlug, modelSlug, settings}
		_ = exports.Set("project", func(call goja.FunctionCall) goja.Value {
			toolName, toolOK := stringArg(call, 0)
			providerName, providerOK := stringArg(call, 1)
			modelName, modelOK := stringArg(call, 2)
			if !toolOK || !providerOK || !modelOK {
				panic(runtime.NewTypeError("osm:userk8s: project requires a tool, a provider, and a model"))
			}
			catalog, err := lazy.get()
			if err != nil {
				panic(runtime.NewTypeError(err.Error()))
			}
			projection, err := catalog.Project(lazy.ctx, toolName, providerName, modelName)
			if err != nil {
				panic(runtime.NewTypeError(err.Error()))
			}
			return toJS(runtime, projection)
		})

		// backendStatus(): backend status counts plus the configured source.
		_ = exports.Set("backendStatus", func(call goja.FunctionCall) goja.Value {
			catalog, err := lazy.get()
			if err != nil {
				panic(runtime.NewTypeError(err.Error()))
			}
			status, err := catalog.Status(lazy.ctx)
			if err != nil {
				panic(runtime.NewTypeError(err.Error()))
			}
			return toJS(runtime, status)
		})
	}
}

// errResolverClass is the only text a resolver failure surfaces: the class
// name, with no resolver output, exit status, or environment detail.
var errResolverClass = errors.New("credential resolver failed")

// catalogError builds a rejected reason with a typed code. The
// resolver-failure-class code never carries the underlying error: resolver
// stdout, stderr, exit status, and environment detail stay in Go. The other
// codes (catalog-not-found, access-not-found) keep the underlying message —
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
	obj := rt.NewGoError(errors.New("This operation was aborted"))
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
// aborting module registration. get() is safe for concurrent callers: load()
// builds the catalog from a tracked-promise goroutine while project() and
// backendStatus() call in synchronously from the loop.
type lazyCatalog struct {
	ctx     context.Context
	options Options
	build   Provider

	mu      sync.Mutex
	catalog userk8s.Catalog
	err     error
}

func (l *lazyCatalog) get() (userk8s.Catalog, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.catalog != nil || l.err != nil {
		return l.catalog, l.err
	}
	if err := validateSource(l.options.Source); err != nil {
		l.err = err
		return nil, l.err
	}
	l.catalog, l.err = l.build(l.ctx, l.options)
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

// LoadedAccess is the JS-facing shape of a ModelAccess. It carries the auth
// requirements but never a credential value.
type LoadedAccess struct {
	Name        string            `json:"name"`
	Registry    string            `json:"registryName"`
	Provider    string            `json:"provider"`
	Mode        string            `json:"mode"`
	Scheme      string            `json:"scheme,omitempty"`
	RequiredEnv []string          `json:"requiredEnv,omitempty"`
	Endpoints   map[string]string `json:"endpoints,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Order       int32             `json:"order"`
	Deprecated  bool              `json:"deprecated,omitempty"`
}

// LoadedModel is the JS-facing shape of a Model.
type LoadedModel struct {
	Name             string   `json:"name"`
	Registry         string   `json:"registryName"`
	Provider         string   `json:"provider"`
	Access           string   `json:"access,omitempty"`
	ContextWindow    int64    `json:"contextWindow"`
	MaxOutputTokens  int64    `json:"maxOutputTokens"`
	CanReason        bool     `json:"canReason"`
	InputModalities  []string `json:"inputModalities,omitempty"`
	ReasoningEfforts []string `json:"reasoningEfforts,omitempty"`
	Default          bool     `json:"default,omitempty"`
	Deprecated       bool     `json:"deprecated,omitempty"`
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
	Accesses       []LoadedAccess    `json:"accesses"`
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
	tools, err := catalog.ListTools(ctx)
	if err != nil {
		return Loaded{}, err
	}

	loaded := Loaded{
		Source:         status.Source,
		Providers:      make([]LoadedProviders, 0, len(providers)),
		Models:         make([]LoadedModel, 0, len(models)),
		Accesses:       make([]LoadedAccess, 0, len(accesses)),
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
			Name:       access.Name,
			Registry:   registryName(access.Name, access.Annotations),
			Provider:   access.Spec.Provider,
			Mode:       access.Spec.Mode,
			Endpoints:  map[string]string{},
			Labels:     map[string]string{},
			Order:      access.Spec.Order,
			Deprecated: access.Spec.Deprecated,
		}
		if access.Spec.Auth != nil {
			view.Scheme = string(access.Spec.Auth.Scheme)
			view.RequiredEnv = append([]string(nil), access.Spec.Auth.RequiredEnv...)
		}
		for surface, endpoint := range access.Spec.Endpoints {
			view.Endpoints[surface] = string(endpoint)
		}
		maps.Copy(view.Labels, access.Labels)
		loaded.Accesses = append(loaded.Accesses, view)
	}
	for i := range models {
		model := &models[i]
		loaded.Models = append(loaded.Models, LoadedModel{
			Name:             model.Name,
			Registry:         registryName(model.Name, model.Annotations),
			Provider:         model.Spec.Provider,
			Access:           model.Spec.Access,
			ContextWindow:    model.Spec.ContextWindow,
			MaxOutputTokens:  model.Spec.MaxOutputTokens,
			CanReason:        model.Spec.CanReason,
			InputModalities:  append([]string(nil), model.Spec.InputModalities...),
			ReasoningEfforts: append([]string(nil), model.Spec.ReasoningEfforts...),
			Default:          model.Spec.Default,
			Deprecated:       model.Spec.Deprecated,
		})
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

// registryName returns the raw registry identifier an object declares, else
// its metadata name.
func registryName(name string, annotations map[string]string) string {
	if registry := annotations[v1alpha1.RegistryNameAnnotation]; registry != "" {
		return registry
	}
	return name
}
