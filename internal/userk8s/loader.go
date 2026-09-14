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
	"k8s.io/apimachinery/pkg/labels"
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
	Providers []v1alpha1.ModelProvider
	Accesses  []v1alpha1.ModelAccess
	Models    []v1alpha1.Model
	Tools     []v1alpha1.Tool
	Bindings  []v1alpha1.LocalSecretBinding

	// Artifacts are the files that contributed documents, in load order.
	Artifacts []string

	// Skipped counts documents ignored because they carry no catalog facts.
	Skipped int

	providerByName map[string]*v1alpha1.ModelProvider
	accessByName   map[string]*v1alpha1.ModelAccess
	modelByName    map[string]*v1alpha1.Model
	toolByName     map[string]*v1alpha1.Tool

	// Registry-name indexes. References inside the catalog are written with
	// raw registry identifiers (for example a model named
	// "deepseek-v4-flash:camel"), while metadata.name must be a DNS-1123
	// label; both spellings resolve to the same object.
	providerByRegistry map[string]*v1alpha1.ModelProvider
	accessByRegistry   map[string]*v1alpha1.ModelAccess
	modelByRegistry    map[string]*v1alpha1.Model
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

// LoadDocuments loads documents supplied in memory; used by tests and by
// in-memory sources.
func LoadDocuments(documents ...[]byte) (*Objects, error) {
	objects := &Objects{}
	for i, document := range documents {
		if err := objects.loadBytes(document, fmt.Sprintf("document[%d]", i)); err != nil {
			return nil, err
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
	o.append(meta.Kind, target)
	return nil
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
	o.toolByName = make(map[string]*v1alpha1.Tool, len(o.Tools))
	o.providerByRegistry = make(map[string]*v1alpha1.ModelProvider, len(o.Providers))
	o.accessByRegistry = make(map[string]*v1alpha1.ModelAccess, len(o.Accesses))
	o.modelByRegistry = make(map[string]*v1alpha1.Model, len(o.Models))

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

	defaults := map[string]string{}
	for i := range o.Models {
		model := &o.Models[i]
		provider, ok := o.lookupProvider(model.Spec.Provider)
		if !ok {
			return fmt.Errorf("Model %q references undeclared ModelProvider %q", model.Name, model.Spec.Provider)
		}
		if model.Spec.Access != "" {
			access, ok := o.lookupAccess(model.Spec.Access)
			if !ok {
				return fmt.Errorf("Model %q references undeclared ModelAccess %q", model.Name, model.Spec.Access)
			}
			accessProvider, ok := o.lookupProvider(access.Spec.Provider)
			if !ok || accessProvider != provider {
				return fmt.Errorf("Model %q declares provider %q but access %q belongs to provider %q", model.Name, model.Spec.Provider, access.Name, access.Spec.Provider)
			}
		}
		if model.Spec.Default {
			key := registryName(provider)
			if existing, ok := defaults[key]; ok {
				return fmt.Errorf("models %q and %q both declare default for provider %q", existing, model.Name, model.Spec.Provider)
			}
			defaults[key] = model.Name
		}
		for _, equivalent := range model.Spec.EquivalentTo {
			peer, ok := o.lookupModel(equivalent.Name)
			if !ok {
				return fmt.Errorf("Model %q equivalent_to references undeclared model %q", model.Name, equivalent.Name)
			}
			peerProvider, ok := o.lookupProvider(peer.Spec.Provider)
			if !ok {
				return fmt.Errorf("Model %q equivalent_to %q belongs to undeclared ModelProvider %q", model.Name, equivalent.Name, peer.Spec.Provider)
			}
			if declaredProvider, ok := o.lookupProvider(equivalent.Provider); !ok || declaredProvider != peerProvider {
				return fmt.Errorf("Model %q equivalent_to %q declares provider %q but that model belongs to provider %q", model.Name, equivalent.Name, equivalent.Provider, peer.Spec.Provider)
			}
			if !equivalentNames(peer).Has(registryName(model)) {
				return fmt.Errorf("equivalent_to must be symmetric: %q lists %q but %q does not list %q", model.Name, peer.Name, peer.Name, model.Name)
			}
		}
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

// equivalentNames returns the equivalence names declared by one model.
func equivalentNames(model *v1alpha1.Model) labels.Set {
	set := labels.Set{}
	for _, equivalent := range model.Spec.EquivalentTo {
		set[equivalent.Name] = ""
	}
	return set
}
