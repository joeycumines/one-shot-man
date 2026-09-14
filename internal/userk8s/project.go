package userk8s

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// registryName is the raw registry identifier of an object: the
// one-shot-man/registry-name annotation when it is set, otherwise
// metadata.name (which equals the registry name when no sanitization was
// needed).
func registryName(object metav1.Object) string {
	if annotations := object.GetAnnotations(); annotations != nil {
		if name := annotations[RegistryNameAnnotation]; name != "" {
			return name
		}
	}
	return object.GetName()
}

// Project derives the tool-facing projection for one selection. Slugs come
// from the registry names of the provider and model, validated against the
// tool's declared identifier grammars; settings are the model's hard facts
// with the model's sparse per-tool overrides applied. The tool's budget
// profile is named, never derived: derivation is adapter code.
// Project derives the tool-facing projection for one string-identified
// selection. Names are read in registry vocabulary (the raw registry
// identifier wins over a sanitized metadata name, because the two can
// collide); in-process callers that already hold a model should prefer
// ProjectModel, which cannot be ambiguous.
func (b *FilesBackend) Project(ctx context.Context, toolName, providerName, modelName string) (Projection, error) {
	tool, model, err := b.resolveSelection(toolName, providerName, modelName)
	if err != nil {
		return Projection{}, err
	}
	if err := ctx.Err(); err != nil {
		return Projection{}, err
	}
	return b.projectResolved(tool, model)
}

// ProjectModel derives the projection for an already-resolved model, so a
// caller iterating the catalog never has to disambiguate between a metadata
// name and another object's registry name.
func (b *FilesBackend) ProjectModel(ctx context.Context, toolName string, model *v1alpha1.Model) (Projection, error) {
	if model == nil {
		return Projection{}, errors.New("userk8s: ProjectModel requires a model")
	}
	if err := ctx.Err(); err != nil {
		return Projection{}, err
	}
	tool, err := b.Tool(toolName)
	if err != nil {
		return Projection{}, err
	}
	return b.projectResolved(tool, model)
}

func (b *FilesBackend) resolveSelection(toolName, providerName, modelName string) (*v1alpha1.Tool, *v1alpha1.Model, error) {
	tool, err := b.Tool(toolName)
	if err != nil {
		return nil, nil, err
	}
	model, err := b.Model(modelName)
	if err != nil {
		return nil, nil, err
	}
	provider, err := b.Provider(providerName)
	if err != nil {
		return nil, nil, err
	}
	modelProvider, ok := b.objects.lookupProvider(model.Spec.Provider)
	if !ok || modelProvider != provider {
		return nil, nil, fmt.Errorf("Model %q belongs to provider %q, not %q", modelName, model.Spec.Provider, providerName)
	}
	return tool, model, nil
}

// projectResolved builds the projection for one resolved tool and model. Slugs
// are the registry names, validated against the tool's declared identifier
// grammars; settings are the model's hard facts with its sparse per-tool
// overrides applied. The tool's budget profile is named, never derived:
// derivation is adapter code.
func (b *FilesBackend) projectResolved(tool *v1alpha1.Tool, model *v1alpha1.Model) (Projection, error) {
	provider, ok := b.objects.lookupProvider(model.Spec.Provider)
	if !ok {
		return Projection{}, fmt.Errorf("Model %q references undeclared ModelProvider %q", model.Name, model.Spec.Provider)
	}

	providerID := registryName(provider)
	modelID := registryName(model)
	providerSlug, err := deriveSlug(tool, "provider", tool.Spec.Identifiers.ProviderSlug, providerID)
	if err != nil {
		return Projection{}, err
	}
	modelSlug, err := deriveSlug(tool, "model", tool.Spec.Identifiers.ModelSlug, modelID)
	if err != nil {
		return Projection{}, err
	}

	projection := Projection{
		ProviderSlug:  providerSlug,
		ModelSlug:     modelSlug,
		ProviderID:    providerID,
		ModelID:       modelID,
		BudgetProfile: tool.Spec.BudgetProfile,
		Settings:      modelSettings(model, tool.Name),
	}
	if model.Spec.Access != "" {
		access, err := b.Access(model.Spec.Access)
		if err != nil {
			return Projection{}, fmt.Errorf("Model %q: %w", model.Name, err)
		}
		projection.Access = access.Name
		projection.Surfaces = servedSurfaces(access, tool)
	}
	return projection, nil
}

// slugDisallowed matches every non-alphanumeric character. The live tool
// configurations spell a derived key by replacing each separator with an
// underscore, which is stricter than any single tool grammar (opencode accepts
// dots and dashes yet its configuration still pairs the key "glm_5_3_dev" with
// the id "glm-5.3:dev").
var slugDisallowed = regexp.MustCompile(`[^A-Za-z0-9]`)

// deriveSlug returns the identifier a tool accepts for one registry name. The
// raw registry spelling is kept whenever the tool's grammar accepts it, so a
// tool that takes raw identifiers keeps them verbatim; otherwise every
// non-alphanumeric character becomes "_", the spelling the live configurations
// use (opencode pairs the key "glm_5_3_dev" with the id "glm-5.3:dev"). A
// grammar the derivation cannot satisfy is an error, never a silent
// substitution.
func deriveSlug(tool *v1alpha1.Tool, class string, grammar v1alpha1.ToolIdentifierGrammar, raw string) (string, error) {
	if grammar.Pattern == "" {
		return "", fmt.Errorf("Tool %q declares no identifiers.%s_slug pattern", tool.Name, class)
	}
	// The declared grammar is anchored: an unanchored pattern would accept a
	// substring of a name the tool cannot actually take.
	pattern, err := regexp.Compile("^(?:" + grammar.Pattern + ")$")
	if err != nil {
		return "", fmt.Errorf("Tool %q identifiers.%s_slug pattern: %w", tool.Name, class, err)
	}
	if pattern.MatchString(raw) {
		return raw, nil
	}
	derived := slugDisallowed.ReplaceAllString(raw, "_")
	if pattern.MatchString(derived) {
		return derived, nil
	}
	return "", fmt.Errorf("Tool %q cannot derive a %s identifier from %q: neither it nor %q matches %s", tool.Name, class, raw, derived, grammar.Pattern)
}

// servedSurfaces is the intersection of the surfaces the tool speaks and the
// surfaces the access serves, sorted for determinism.
func servedSurfaces(access *v1alpha1.ModelAccess, tool *v1alpha1.Tool) []string {
	var surfaces []string
	for _, surface := range tool.Spec.Surfaces {
		if _, ok := access.Spec.Endpoints[surface]; ok {
			surfaces = append(surfaces, surface)
		}
	}
	sort.Strings(surfaces)
	return surfaces
}

// modelSettings is the model's fact sheet with sparse per-tool overrides
// applied. Only facts and overrides are emitted; nothing is derived.
func modelSettings(model *v1alpha1.Model, tool string) map[string]any {
	settings := map[string]any{
		"context_window":    model.Spec.ContextWindow,
		"max_output_tokens": model.Spec.MaxOutputTokens,
		"can_reason":        model.Spec.CanReason,
		"input_modalities":  append([]string(nil), model.Spec.InputModalities...),
	}
	if len(model.Spec.ReasoningEfforts) > 0 {
		settings["reasoning_efforts"] = append([]string(nil), model.Spec.ReasoningEfforts...)
	}
	if model.Spec.Default {
		settings["default"] = true
	}
	if model.Spec.Deprecated {
		settings["deprecated"] = true
	}

	override, ok := model.Spec.ToolSettings[tool]
	if !ok {
		return settings
	}
	if override.ContextWindow != nil {
		settings["context_window"] = *override.ContextWindow
	}
	if override.MaxOutputTokens != nil {
		settings["max_output_tokens"] = *override.MaxOutputTokens
	}
	if override.AutoCompactWindow != nil {
		settings["auto_compact_window"] = *override.AutoCompactWindow
	}
	if override.EffortLevel != nil {
		settings["effort_level"] = *override.EffortLevel
	}
	if len(override.Flags) > 0 {
		flags := make(map[string]string, len(override.Flags))
		for key, value := range override.Flags {
			flags[key] = value
		}
		settings["flags"] = flags
	}
	if override.Waive != "" {
		settings["waive"] = override.Waive
	}
	return settings
}
