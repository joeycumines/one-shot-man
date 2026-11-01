package userk8s

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GroupVersion is the apiVersion every catalog document must declare.
const GroupVersion = "one-shot-man/v1alpha1"

// RegistryNameAnnotation carries the raw registry identifier of an object
// whose metadata.name was sanitized to a DNS-1123 label.
const RegistryNameAnnotation = "one-shot-man/registry-name"

// ignoredKinds are rendered for cluster apply alongside the catalog and carry
// no catalog facts: the local backend skips them by design.
var ignoredKinds = map[string]bool{
	"Namespace": true,
	"Secret":    true,
}

// Objects is the indexed, cross-checked content of one rendered profile.
type Objects struct {
	Providers  []v1alpha1.ModelProvider
	Accesses   []v1alpha1.ModelAccess
	Models     []v1alpha1.Model
	Servings   []v1alpha1.ModelServing
	Selections []v1alpha1.ModelSelection
	Routes     []v1alpha1.ModelRoute
	Tools      []v1alpha1.Tool
	Bindings   []v1alpha1.LocalSecretBinding

	// Artifacts are the files that contributed documents, in load order.
	Artifacts []string

	// Skipped counts documents ignored because they carry no catalog facts.
	Skipped int

	providerByName  map[string]*v1alpha1.ModelProvider
	accessByName    map[string]*v1alpha1.ModelAccess
	modelByName     map[string]*v1alpha1.Model
	servingByName   map[string]*v1alpha1.ModelServing
	selectionByName map[string]*v1alpha1.ModelSelection
	routeByName     map[string]*v1alpha1.ModelRoute
	toolByName      map[string]*v1alpha1.Tool

	// Registry-name indexes. References inside the catalog are written with
	// raw registry identifiers (for example a model named
	// "deepseek-v4-flash:camel"), while metadata.name must be a DNS-1123
	// label; both spellings resolve to the same object.
	providerByRegistry  map[string]*v1alpha1.ModelProvider
	accessByRegistry    map[string]*v1alpha1.ModelAccess
	modelByRegistry     map[string]*v1alpha1.Model
	servingByRegistry   map[string]*v1alpha1.ModelServing
	selectionByRegistry map[string]*v1alpha1.ModelSelection
	routeByRegistry     map[string]*v1alpha1.ModelRoute
}

// typeMeta is the discriminator read before decoding a document fully.
type typeMeta struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// LoadPaths loads every document from each path, then cross-checks the result.
// A directory contributes its *.yaml and *.yml files sorted by name; a file is
// read directly. Paths are deduplicated so a catalog is never double-loaded.
func LoadPaths(paths ...string) (*Objects, error) {
	objects := &Objects{}
	seen := map[string]bool{}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true

		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("catalog path %q: %w", path, err)
		}
		if !info.IsDir() {
			if err := objects.loadFile(path); err != nil {
				return nil, err
			}
			continue
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("catalog directory %q: %w", path, err)
		}
		var files []string
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
				files = append(files, filepath.Join(path, name))
			}
		}
		sort.Strings(files)
		for _, file := range files {
			if err := objects.loadFile(file); err != nil {
				return nil, err
			}
		}
	}
	if err := objects.index(); err != nil {
		return nil, err
	}
	return objects, nil
}

// loadFile loads every document of one file.
func (o *Objects) loadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("catalog file %q: %w", path, err)
	}
	o.Artifacts = append(o.Artifacts, path)
	return o.loadBytes(data, path)
}

// loadBytes parses a multi-document YAML stream with goccy/go-yaml and adds
// each recognized document. Parsing once yields per-document AST nodes, so a
// block scalar or anchor containing a document marker cannot be mistaken for a
// document boundary.
func (o *Objects) loadBytes(data []byte, source string) error {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	file, err := parser.ParseBytes(data, parser.Mode(0))
	if err != nil {
		return fmt.Errorf("catalog %q: %w", source, err)
	}
	for _, document := range file.Docs {
		if document == nil || document.Body == nil {
			continue
		}
		body, ok := document.Body.(*ast.MappingNode)
		if !ok || len(body.Values) == 0 {
			// An empty document (`---` with nothing under it) carries no facts.
			continue
		}
		if err := o.addDocument(body, source); err != nil {
			return err
		}
	}
	return nil
}

// addDocument decodes one document into its indexed collection.
func (o *Objects) addDocument(node *ast.MappingNode, source string) error {
	var meta typeMeta
	if err := yaml.NodeToValue(node, &meta, yaml.UseJSONUnmarshaler()); err != nil {
		return fmt.Errorf("catalog %q: %w", source, err)
	}
	if meta.Kind == "" {
		return fmt.Errorf("catalog %q: document has no kind", source)
	}
	if ignoredKinds[meta.Kind] {
		o.Skipped++
		return nil
	}
	if meta.APIVersion != GroupVersion {
		return fmt.Errorf("catalog %q: %s declares apiVersion %q, want %q", source, meta.Kind, meta.APIVersion, GroupVersion)
	}

	target, err := newTarget(meta.Kind)
	if err != nil {
		return fmt.Errorf("catalog %q: %w", source, err)
	}
	if err := yaml.NodeToValue(node, target, yaml.UseJSONUnmarshaler()); err != nil {
		return fmt.Errorf("catalog %q: decoding %s: %w", source, meta.Kind, err)
	}
	// The API server applies every +kubebuilder:default before an object reaches
	// a backend. Nothing applies them to a file, so the files backend does it
	// here: a document decoded from disk must be indistinguishable from the same
	// object read from a cluster, or the two backends resolve credentials and
	// report capabilities differently.
	defaults, err := decodeDefaults(node)
	if err != nil {
		return fmt.Errorf("catalog %q: reading %s defaults: %w", source, meta.Kind, err)
	}
	if err := applyDefaults(meta.Kind, target, defaults); err != nil {
		return fmt.Errorf("catalog %q: defaulting %s: %w", source, meta.Kind, err)
	}
	o.append(meta.Kind, target)
	return nil
}

// decodeDefaults returns the document's spec as a generic map, which is how the
// defaulting pass knows which fields the author actually wrote: an absent field
// takes its CRD default while an explicit zero value does not. A document with
// no spec yields an empty map.
func decodeDefaults(node *ast.MappingNode) (map[string]any, error) {
	var document map[string]any
	if err := yaml.NodeToValue(node, &document); err != nil {
		return nil, err
	}
	spec, ok := document["spec"].(map[string]any)
	if !ok {
		return map[string]any{}, nil
	}
	return spec, nil
}

// applyDefaults fills the CRD defaults of one catalog kind. Every default here
// is one the API server would apply to the same object; a field the author wrote
// (including an explicit zero) is left alone. Keeping this list beside the type
// markers it mirrors is deliberate: a default added to a type without an entry
// here is the divergence this function exists to prevent, and the parity test in
// userk8s fails when the two disagree.
func applyDefaults(kind string, target any, spec map[string]any) error {
	switch typed := target.(type) {
	case *v1alpha1.Model:
		applyModelDefaults(&typed.Spec, spec)
	case *v1alpha1.ModelServing:
		applyServingDefaults(&typed.Spec, spec)
	case *v1alpha1.ModelRoute:
		applyRouteDefaults(&typed.Spec, spec)
	case *v1alpha1.LocalSecretBinding:
		applyBindingDefaults(&typed.Spec, spec)
	case *v1alpha1.ModelSelection:
		applySelectionDefaults(&typed.Spec, spec)
	case *v1alpha1.ModelProvider, *v1alpha1.ModelAccess, *v1alpha1.Tool:
		// Their defaults are all zero values, which decoding already produced.
	default:
		return fmt.Errorf("kind %T has no defaulting rule; add one or classify it as default-free", target)
	}
	return nil
}

// present reports whether the author wrote a key into the document.
func present(spec map[string]any, key string) bool {
	_, ok := spec[key]
	return ok
}

// applyModelDefaults applies ModelSpec's CRD defaults: tool_call is true and the
// modalities default to text. All three are +kubebuilder:default markers on the
// type; see applyDefaults.
func applyModelDefaults(spec *v1alpha1.ModelSpec, document map[string]any) {
	if !present(document, "tool_call") {
		spec.ToolCall = true
	}
	if !present(document, "input_modalities") {
		spec.InputModalities = []string{"text"}
	}
	if !present(document, "output_modalities") {
		spec.OutputModalities = []string{"text"}
	}
}

// applyServingDefaults applies ModelServingSpec's priority default.
func applyServingDefaults(spec *v1alpha1.ModelServingSpec, document map[string]any) {
	if !present(document, "priority") {
		spec.Priority = 100
	}
}

// applyRouteDefaults applies ModelRouteSpec's order default.
func applyRouteDefaults(spec *v1alpha1.ModelRouteSpec, document map[string]any) {
	if !present(document, "order") {
		spec.Order = 100
	}
}

// applyBindingDefaults applies LocalSecretBindingSpec's defaults: the scheme, and
// the per-resolver command timeout. A credentialless binding says so explicitly
// (scheme: none); omitting the scheme means the value is presented as a bearer
// credential, exactly as it would be from a cluster. The timeout default keeps a
// binding decoded from disk identical to one the API server defaulted, so the
// assembled object never depends on which backend produced it.
func applyBindingDefaults(spec *v1alpha1.LocalSecretBindingSpec, document map[string]any) {
	if !present(document, "scheme") {
		spec.Scheme = v1alpha1.CredentialBearer
	}
	steps, _ := document["resolvers"].([]any)
	for i := range spec.Resolvers {
		command := spec.Resolvers[i].Command
		if command == nil || command.Timeout != nil {
			continue
		}
		// Only default a step whose document entry omitted the timeout; an
		// explicit "0s" is the author's own value.
		if i < len(steps) {
			if entry, ok := steps[i].(map[string]any); ok {
				if commandDoc, ok := entry["command"].(map[string]any); ok {
					if present(commandDoc, "timeout") {
						continue
					}
				}
			}
		}
		timeout := metav1.Duration{Duration: defaultCommandTimeout}
		command.Timeout = &timeout
	}
}

// applySelectionDefaults applies ModelSelectionSpec's strategy default.
func applySelectionDefaults(spec *v1alpha1.ModelSelectionSpec, document map[string]any) {
	if !present(document, "strategy") {
		spec.Strategy = v1alpha1.SelectionOrdered
	}
}

// newTarget returns the address of a fresh value of one kind, or an error when
// the kind is not part of the catalog.
func newTarget(kind string) (any, error) {
	switch kind {
	case "ModelProvider":
		return &v1alpha1.ModelProvider{}, nil
	case "ModelAccess":
		return &v1alpha1.ModelAccess{}, nil
	case "Model":
		return &v1alpha1.Model{}, nil
	case "ModelServing":
		return &v1alpha1.ModelServing{}, nil
	case "ModelSelection":
		return &v1alpha1.ModelSelection{}, nil
	case "ModelRoute":
		return &v1alpha1.ModelRoute{}, nil
	case "Tool":
		return &v1alpha1.Tool{}, nil
	case "LocalSecretBinding":
		return &v1alpha1.LocalSecretBinding{}, nil
	default:
		return nil, fmt.Errorf("kind %q is not a known catalog kind", kind)
	}
}

func (o *Objects) append(kind string, value any) {
	switch typed := value.(type) {
	case *v1alpha1.ModelProvider:
		o.Providers = append(o.Providers, *typed)
	case *v1alpha1.ModelAccess:
		o.Accesses = append(o.Accesses, *typed)
	case *v1alpha1.Model:
		o.Models = append(o.Models, *typed)
	case *v1alpha1.ModelServing:
		o.Servings = append(o.Servings, *typed)
	case *v1alpha1.ModelSelection:
		o.Selections = append(o.Selections, *typed)
	case *v1alpha1.ModelRoute:
		o.Routes = append(o.Routes, *typed)
	case *v1alpha1.Tool:
		o.Tools = append(o.Tools, *typed)
	case *v1alpha1.LocalSecretBinding:
		o.Bindings = append(o.Bindings, *typed)
	}
}

// index builds name indexes, rejects duplicate names, and validates every
// cross-reference the prior engine enforced at compile time.
func (o *Objects) index() error {
	o.providerByName = make(map[string]*v1alpha1.ModelProvider, len(o.Providers))
	o.accessByName = make(map[string]*v1alpha1.ModelAccess, len(o.Accesses))
	o.modelByName = make(map[string]*v1alpha1.Model, len(o.Models))
	o.servingByName = make(map[string]*v1alpha1.ModelServing, len(o.Servings))
	o.selectionByName = make(map[string]*v1alpha1.ModelSelection, len(o.Selections))
	o.routeByName = make(map[string]*v1alpha1.ModelRoute, len(o.Routes))
	o.toolByName = make(map[string]*v1alpha1.Tool, len(o.Tools))
	o.providerByRegistry = make(map[string]*v1alpha1.ModelProvider, len(o.Providers))
	o.accessByRegistry = make(map[string]*v1alpha1.ModelAccess, len(o.Accesses))
	o.modelByRegistry = make(map[string]*v1alpha1.Model, len(o.Models))
	o.servingByRegistry = make(map[string]*v1alpha1.ModelServing, len(o.Servings))
	o.selectionByRegistry = make(map[string]*v1alpha1.ModelSelection, len(o.Selections))
	o.routeByRegistry = make(map[string]*v1alpha1.ModelRoute, len(o.Routes))

	for i := range o.Providers {
		provider := &o.Providers[i]
		if err := indexObject("ModelProvider", provider.Name, registryName(provider), o.providerByName, o.providerByRegistry, provider); err != nil {
			return err
		}
	}
	for i := range o.Accesses {
		access := &o.Accesses[i]
		if err := indexObject("ModelAccess", access.Name, registryName(access), o.accessByName, o.accessByRegistry, access); err != nil {
			return err
		}
	}
	for i := range o.Models {
		model := &o.Models[i]
		if err := indexObject("Model", model.Name, registryName(model), o.modelByName, o.modelByRegistry, model); err != nil {
			return err
		}
	}
	for i := range o.Servings {
		serving := &o.Servings[i]
		if err := indexObject("ModelServing", serving.Name, registryName(serving), o.servingByName, o.servingByRegistry, serving); err != nil {
			return err
		}
	}
	for i := range o.Selections {
		selection := &o.Selections[i]
		if err := indexObject("ModelSelection", selection.Name, registryName(selection), o.selectionByName, o.selectionByRegistry, selection); err != nil {
			return err
		}
	}
	for i := range o.Routes {
		route := &o.Routes[i]
		if err := indexObject("ModelRoute", route.Name, registryName(route), o.routeByName, o.routeByRegistry, route); err != nil {
			return err
		}
	}
	for i := range o.Tools {
		tool := &o.Tools[i]
		if tool.Name == "" {
			return errors.New("Tool with empty metadata.name")
		}
		if _, duplicate := o.toolByName[tool.Name]; duplicate {
			return fmt.Errorf("duplicate Tool %q", tool.Name)
		}
		o.toolByName[tool.Name] = tool
	}
	return o.validate()
}

// indexObject registers one object under its metadata name and, when it
// differs, under its raw registry name, rejecting duplicates of either.
func indexObject[T any](kind, name, registry string, byName, byRegistry map[string]*T, object *T) error {
	if name == "" {
		return fmt.Errorf("%s with empty metadata.name", kind)
	}
	if _, duplicate := byName[name]; duplicate {
		return fmt.Errorf("duplicate %s %q", kind, name)
	}
	byName[name] = object
	if registry == "" || registry == name {
		return nil
	}
	if _, duplicate := byRegistry[registry]; duplicate {
		return fmt.Errorf("duplicate %s registry name %q", kind, registry)
	}
	byRegistry[registry] = object
	return nil
}

// lookupProvider, lookupAccess, and lookupModel resolve a reference written
// with either the raw registry name or the metadata name. Registry names win:
// references are authored in registry vocabulary, and a sanitized metadata
// name can collide with another object's registry name (the emitter suffixes
// such a metadata name, e.g. "deepseek-v4-pro-2").
func (o *Objects) lookupProvider(ref string) (*v1alpha1.ModelProvider, bool) {
	if provider, ok := o.providerByRegistry[ref]; ok {
		return provider, true
	}
	provider, ok := o.providerByName[ref]
	return provider, ok
}

func (o *Objects) lookupAccess(ref string) (*v1alpha1.ModelAccess, bool) {
	if access, ok := o.accessByRegistry[ref]; ok {
		return access, true
	}
	access, ok := o.accessByName[ref]
	return access, ok
}

func (o *Objects) lookupModel(ref string) (*v1alpha1.Model, bool) {
	if model, ok := o.modelByRegistry[ref]; ok {
		return model, true
	}
	model, ok := o.modelByName[ref]
	return model, ok
}

// lookupBinding resolves a LocalSecretBinding by metadata name. Bindings are
// namespaced; the backend loads one namespace (files: one profile), so a name
// is unambiguous here.
func (o *Objects) lookupBinding(ref string) (*v1alpha1.LocalSecretBinding, bool) {
	for i := range o.Bindings {
		if o.Bindings[i].Name == ref {
			return &o.Bindings[i], true
		}
	}
	return nil, false
}

// lookupServing resolves a ModelServing by registry name or metadata name.
func (o *Objects) lookupServing(ref string) (*v1alpha1.ModelServing, bool) {
	if serving, ok := o.servingByRegistry[ref]; ok {
		return serving, true
	}
	serving, ok := o.servingByName[ref]
	return serving, ok
}

// lookupSelection resolves a ModelSelection by registry name or metadata name.
func (o *Objects) lookupSelection(ref string) (*v1alpha1.ModelSelection, bool) {
	if selection, ok := o.selectionByRegistry[ref]; ok {
		return selection, true
	}
	selection, ok := o.selectionByName[ref]
	return selection, ok
}

// lookupRoute resolves a route reference written with either its registry name
// or its metadata name.
func (o *Objects) lookupRoute(ref string) (*v1alpha1.ModelRoute, bool) {
	if route, ok := o.routeByRegistry[ref]; ok {
		return route, true
	}
	route, ok := o.routeByName[ref]
	return route, ok
}

// validate ports the fail-fast cross-checks that previously lived in
// compile-configs.py: dangling references, provider/access disagreement,
// equivalence symmetry, per-provider default uniqueness, empty binding
// selectors, and uncompilable tool grammars.
func (o *Objects) validate() error {
	for i := range o.Accesses {
		access := &o.Accesses[i]
		if _, ok := o.lookupProvider(access.Spec.Provider); !ok {
			return fmt.Errorf("ModelAccess %q references undeclared ModelProvider %q", access.Name, access.Spec.Provider)
		}
	}

	if err := o.validateAccesses(); err != nil {
		return err
	}
	if err := o.validateModels(); err != nil {
		return err
	}
	if err := o.validateServings(); err != nil {
		return err
	}
	if err := o.validateSelections(); err != nil {
		return err
	}

	if err := o.validateRoutes(); err != nil {
		return err
	}

	for i := range o.Bindings {
		binding := &o.Bindings[i]
		if len(binding.Spec.Selector.MatchLabels) == 0 && len(binding.Spec.Selector.MatchExpressions) == 0 {
			return fmt.Errorf("LocalSecretBinding %q declares an empty selector, which would match every access", binding.Name)
		}
		if _, err := metav1.LabelSelectorAsSelector(&binding.Spec.Selector); err != nil {
			return fmt.Errorf("LocalSecretBinding %q: %w", binding.Name, err)
		}
	}

	for i := range o.Tools {
		tool := &o.Tools[i]
		for _, grammar := range []struct {
			field   string
			pattern string
		}{
			{"provider_slug", tool.Spec.Identifiers.ProviderSlug.Pattern},
			{"model_slug", tool.Spec.Identifiers.ModelSlug.Pattern},
		} {
			if _, err := regexp.Compile(grammar.pattern); err != nil {
				return fmt.Errorf("Tool %q identifiers.%s: %w", tool.Name, grammar.field, err)
			}
		}
	}
	return nil
}
