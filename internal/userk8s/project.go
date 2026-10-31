package userk8s

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"

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

// Project derives the tool-facing projection for one selected serving. Names
// are read in registry vocabulary (the raw registry identifier wins over a
// sanitized metadata name, because the two can collide); in-process callers
// that already hold a serving should prefer ProjectServing.
func (b *FilesBackend) Project(ctx context.Context, toolName, providerName, servingName string) (Projection, error) {
	tool, serving, err := b.resolveSelection(toolName, providerName, servingName)
	if err != nil {
		return Projection{}, err
	}
	if err := ctx.Err(); err != nil {
		return Projection{}, err
	}
	return b.projectResolved(tool, serving)
}

// ProjectServing derives the projection for an already-resolved serving, so a
// caller iterating the catalog never has to disambiguate a name.
func (b *FilesBackend) ProjectServing(ctx context.Context, toolName string, serving *v1alpha1.ModelServing) (Projection, error) {
	if serving == nil {
		return Projection{}, errors.New("userk8s: ProjectServing requires a serving")
	}
	if err := ctx.Err(); err != nil {
		return Projection{}, err
	}
	tool, err := b.Tool(toolName)
	if err != nil {
		return Projection{}, err
	}
	return b.projectResolved(tool, serving)
}

func (b *FilesBackend) resolveSelection(toolName, providerName, servingName string) (*v1alpha1.Tool, *v1alpha1.ModelServing, error) {
	tool, err := b.Tool(toolName)
	if err != nil {
		return nil, nil, err
	}
	serving, ok := b.objects.lookupServing(servingName)
	if !ok {
		return nil, nil, fmt.Errorf("unknown ModelServing %q", servingName)
	}
	provider, err := b.Provider(providerName)
	if err != nil {
		return nil, nil, err
	}
	servingProvider, ok := b.objects.lookupProvider(serving.Spec.Provider)
	if !ok || servingProvider != provider {
		return nil, nil, fmt.Errorf("ModelServing %q belongs to provider %q, not %q", servingName, serving.Spec.Provider, providerName)
	}
	return tool, serving, nil
}

// projectResolved builds the projection for one resolved tool and serving.
// Slugs are the registry names, validated against the tool's declared
// identifier grammars; settings are the effective capabilities (Model plus
// serving narrowings) with the Model's sparse per-tool overrides applied. The
// tool's budget profile is named, never derived: derivation is adapter code.
func (b *FilesBackend) projectResolved(tool *v1alpha1.Tool, serving *v1alpha1.ModelServing) (Projection, error) {
	provider, ok := b.objects.lookupProvider(serving.Spec.Provider)
	if !ok {
		return Projection{}, fmt.Errorf("ModelServing %q references undeclared ModelProvider %q", serving.Name, serving.Spec.Provider)
	}
	model, ok := b.objects.lookupModel(serving.Spec.Model)
	if !ok {
		return Projection{}, fmt.Errorf("ModelServing %q references undeclared Model %q", serving.Name, serving.Spec.Model)
	}
	effective, err := effectiveModel(model, serving)
	if err != nil {
		return Projection{}, err
	}

	providerID := registryName(provider)
	modelID := registryName(serving)
	providerSlug, err := deriveSlug(tool, "provider", tool.Spec.Identifiers.ProviderSlug, providerID)
	if err != nil {
		return Projection{}, err
	}
	modelSlug, err := deriveSlug(tool, "model", tool.Spec.Identifiers.ModelSlug, modelID)
	if err != nil {
		return Projection{}, err
	}

	return Projection{
		ProviderSlug:    providerSlug,
		ModelSlug:       modelSlug,
		ProviderID:      providerID,
		ModelID:         modelID,
		ProviderModelID: serving.Spec.ProviderModelID,
		BudgetProfile:   tool.Spec.BudgetProfile,
		Settings:        modelSettings(model, serving, effective, tool.Name),
	}, nil
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

// modelSettings is the model's fact sheet with sparse per-tool overrides
// applied. Only facts and overrides are emitted; nothing is derived.
func modelSettings(model *v1alpha1.Model, serving *v1alpha1.ModelServing, effective EffectiveModel, tool string) map[string]any {
	settings := map[string]any{
		"context_window":    effective.ContextWindow,
		"max_output_tokens": effective.MaxOutputTokens,
		"input_modalities":  append([]string(nil), effective.InputModalities...),
		"output_modalities": append([]string(nil), effective.OutputModalities...),
		"tool_call":         effective.ToolCall,
		"structured_output": effective.StructuredOutput,
	}
	if reasoning := effective.Reasoning; reasoning != nil {
		rendered := map[string]any{"supported": reasoning.Supported}
		if reasoning.Toggle {
			rendered["toggle"] = true
		}
		if len(reasoning.Efforts) > 0 {
			rendered["efforts"] = append([]string(nil), reasoning.Efforts...)
		}
		if reasoning.Budget != nil {
			budget := map[string]any{}
			if reasoning.Budget.Min != nil {
				budget["min"] = *reasoning.Budget.Min
			}
			if reasoning.Budget.Max != nil {
				budget["max"] = *reasoning.Budget.Max
			}
			rendered["budget"] = budget
		}
		settings["reasoning"] = rendered
	}
	settings["priority"] = serving.Spec.EffectivePriority()
	if serving.Spec.Default {
		settings["default"] = true
	}
	if model.Spec.Deprecated || serving.Spec.Deprecated {
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
		maps.Copy(flags, override.Flags)
		settings["flags"] = flags
	}
	if override.Waive != "" {
		settings["waive"] = override.Waive
	}
	return settings
}
