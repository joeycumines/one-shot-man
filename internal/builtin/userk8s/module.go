// Package userk8smod provides the Goja module registered as "osm:userk8s":
// the typed catalog API over the userk8s engine. It exposes the model
// catalog the launcher and tool adapters read, resolves an access's
// credentials through its selector-matched LocalSecretBindings, and projects a
// (tool, provider, model) selection into the slug pair and settings a tool
// adapter launches with.
//
// Credential values cross into JS only as the value field of a resolved
// credential; nothing else in the surface exposes credential material, and
// resolver failures are reported as classes rather than error text.
package userk8smod

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/joeycumines/goja"
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
// JavaScript-visible difference.
func Require(ctx context.Context, options Options) func(runtime *goja.Runtime, module *goja.Object) {
	build := filesProvider
	if options.Source == SourceCluster {
		build = clusterProvider
	}
	return RequireWithProvider(ctx, options, build)
}

// RequireWithProvider is Require with an injectable backend, so the cluster
// backend can be wired without this package importing client-go.
func RequireWithProvider(ctx context.Context, options Options, build Provider) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)
		lazy := &lazyCatalog{ctx: ctx, options: options, build: build}

		// load(): {providers, models, accesses, tools, secretsPresent}
		_ = exports.Set("load", func(call goja.FunctionCall) goja.Value {
			catalog, err := lazy.get()
			if err != nil {
				panic(runtime.NewTypeError(err.Error()))
			}
			loaded, err := loadCatalog(lazy.ctx, catalog)
			if err != nil {
				panic(runtime.NewTypeError(err.Error()))
			}
			return toJS(runtime, loaded)
		})

		// resolveCredential(accessRef): {status, reason?, credentials}
		_ = exports.Set("resolveCredential", func(call goja.FunctionCall) goja.Value {
			accessRef, ok := stringArg(call, 0)
			if !ok {
				panic(runtime.NewTypeError("osm:userk8s: resolveCredential requires an access reference"))
			}
			catalog, err := lazy.get()
			if err != nil {
				panic(runtime.NewTypeError(err.Error()))
			}
			resolution, err := catalog.ResolveCredential(lazy.ctx, accessRef)
			if err != nil {
				panic(runtime.NewTypeError(err.Error()))
			}
			return toJS(runtime, resolution)
		})

		// project(toolName, providerName, modelName): {providerSlug, modelSlug, settings}
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
// aborting module registration.
type lazyCatalog struct {
	ctx     context.Context
	options Options
	build   Provider
	catalog userk8s.Catalog
	err     error
}

func (l *lazyCatalog) get() (userk8s.Catalog, error) {
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
		for key, value := range access.Labels {
			view.Labels[key] = value
		}
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
