package userk8s

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

// This file mirrors, in Go, the invariants the CRD expresses in CEL. The files
// backend decodes YAML directly and never runs an API server, so a rule that
// only exists as a +kubebuilder marker is not enforced on the path that reads a
// rendered artifact. Every check here has a CEL counterpart in
// api/v1alpha1/*_types.go; adding one without the other reintroduces exactly the
// divergence these checks exist to close.

// reasoningEfforts is the effort vocabulary the CRD enumerates.
var reasoningEfforts = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"}

// modalityVocabulary is the modality vocabulary the CRD enumerates.
var modalityVocabulary = []string{"text", "image", "audio"}

// routeOfferIDPattern is the served-id grammar. A served id is an RFC-1123
// name, the same as a metadata name.
var routeOfferIDPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// validateModels enforces ModelSpec's CEL rules: required identity and budgets,
// the modality vocabulary, the reasoning fact sheet, and the tool settings that
// may exceed a fact only behind a waive reason.
func (o *Objects) validateModels() error {
	for i := range o.Models {
		if err := validateModel(&o.Models[i]); err != nil {
			return err
		}
	}
	return nil
}

func validateModel(model *v1alpha1.Model) error {
	spec := &model.Spec
	if spec.CanonicalID == "" {
		return fmt.Errorf("Model %q declares no canonical_id", model.Name)
	}
	if spec.DisplayName == "" {
		return fmt.Errorf("Model %q declares no display_name", model.Name)
	}
	if spec.ContextWindow <= 0 {
		return fmt.Errorf("Model %q declares context_window %d; it must be positive", model.Name, spec.ContextWindow)
	}
	if spec.MaxOutputTokens <= 0 {
		return fmt.Errorf("Model %q declares max_output_tokens %d; it must be positive", model.Name, spec.MaxOutputTokens)
	}
	if err := validateModalities("input", model.Name, spec.InputModalities); err != nil {
		return err
	}
	if err := validateModalities("output", model.Name, spec.OutputModalities); err != nil {
		return err
	}
	if spec.Reasoning != nil {
		if err := validateReasoningFactSheet(model.Name, spec.Reasoning); err != nil {
			return err
		}
	}
	return validateToolSettings(model)
}

// validateModalities checks the vocabulary and rejects a duplicate, matching the
// CEL enum and UniqueItems markers.
func validateModalities(kind, modelName string, modalities []string) error {
	seen := map[string]bool{}
	for _, modality := range modalities {
		if !slices.Contains(modalityVocabulary, modality) {
			return fmt.Errorf("Model %q declares %s modality %q; it must be one of %v", modelName, kind, modality, modalityVocabulary)
		}
		if seen[modality] {
			return fmt.Errorf("Model %q declares %s modality %q twice", modelName, kind, modality)
		}
		seen[modality] = true
	}
	return nil
}

// validateReasoningFactSheet enforces ModelReasoning's invariants: a mode
// requires supported: true, efforts come from the enumerated vocabulary, and a
// budget range is non-negative, positive where bounded, and ordered.
func validateReasoningFactSheet(modelName string, reasoning *v1alpha1.ModelReasoning) error {
	if !reasoning.Supported && (reasoning.Toggle || len(reasoning.Efforts) > 0 || reasoning.Budget != nil) {
		return fmt.Errorf("Model %q declares a reasoning mode but does not set supported: true", modelName)
	}
	seen := map[string]bool{}
	for _, effort := range reasoning.Efforts {
		if !slices.Contains(reasoningEfforts, effort) {
			return fmt.Errorf("Model %q declares reasoning effort %q; it must be one of %v", modelName, effort, reasoningEfforts)
		}
		if seen[effort] {
			return fmt.Errorf("Model %q declares reasoning effort %q twice", modelName, effort)
		}
		seen[effort] = true
	}
	return validateReasoningBudget(modelName, reasoning.Budget)
}

// validateReasoningBudget enforces the budget range on its own, so a serving's
// override can share it.
func validateReasoningBudget(modelName string, budget *v1alpha1.ModelReasoningBudget) error {
	if budget == nil {
		return nil
	}
	if budget.Min != nil && *budget.Min < 0 {
		return fmt.Errorf("Model %q declares reasoning budget min %d; it must be non-negative", modelName, *budget.Min)
	}
	if budget.Max != nil && *budget.Max <= 0 {
		return fmt.Errorf("Model %q declares reasoning budget max %d; it must be positive", modelName, *budget.Max)
	}
	if budget.Min != nil && budget.Max != nil && *budget.Min > *budget.Max {
		return fmt.Errorf("Model %q declares reasoning budget min %d above its max %d", modelName, *budget.Min, *budget.Max)
	}
	return nil
}

// validateToolSettings enforces the CEL rules on ModelSpec.ToolSettings: an
// override must be positive, and one that exceeds the model's fact is a
// deliberate compromise that requires a waive reason. The effort level must name
// a level the model publishes.
func validateToolSettings(model *v1alpha1.Model) error {
	spec := &model.Spec
	tools := make([]string, 0, len(spec.ToolSettings))
	for tool := range spec.ToolSettings {
		tools = append(tools, tool)
	}
	slices.Sort(tools)
	for _, tool := range tools {
		settings := spec.ToolSettings[tool]
		for _, bound := range []struct {
			field string
			value *int64
			fact  int64
			label string
		}{
			{"context_window", settings.ContextWindow, spec.ContextWindow, "context_window"},
			{"max_output_tokens", settings.MaxOutputTokens, spec.MaxOutputTokens, "max_output_tokens"},
			{"auto_compact_window", settings.AutoCompactWindow, spec.ContextWindow, "context_window"},
		} {
			if bound.value == nil {
				continue
			}
			if *bound.value <= 0 {
				return fmt.Errorf("Model %q toolSettings[%q].%s must be positive, got %d", model.Name, tool, bound.field, *bound.value)
			}
			if *bound.value > bound.fact && settings.Waive == "" {
				return fmt.Errorf("Model %q toolSettings[%q].%s %d exceeds its %s %d without a waive reason", model.Name, tool, bound.field, *bound.value, bound.label, bound.fact)
			}
		}
		if settings.EffortLevel == nil {
			continue
		}
		if !slices.Contains(reasoningEfforts, *settings.EffortLevel) {
			return fmt.Errorf("Model %q toolSettings[%q].effort_level %q must be one of %v", model.Name, tool, *settings.EffortLevel, reasoningEfforts)
		}
		if spec.Reasoning != nil && len(spec.Reasoning.Efforts) > 0 && !slices.Contains(spec.Reasoning.Efforts, *settings.EffortLevel) {
			return fmt.Errorf("Model %q toolSettings[%q].effort_level %q is not one of the model's reasoning efforts %v", model.Name, tool, *settings.EffortLevel, spec.Reasoning.Efforts)
		}
	}
	return nil
}

// validateAccesses enforces ModelAccessSpec's protocol enum for every access,
// not only the ones a route happens to reference: an unreferenced access is
// still a catalog statement, and a typo in it must not load silently.
func (o *Objects) validateAccesses() error {
	for i := range o.Accesses {
		access := &o.Accesses[i]
		switch access.Spec.Protocol {
		case v1alpha1.WireProtocolChat, v1alpha1.WireProtocolMessages, v1alpha1.WireProtocolResponses:
		case "":
			return fmt.Errorf("ModelAccess %q declares no protocol", access.Name)
		default:
			return fmt.Errorf("ModelAccess %q declares unknown protocol %q; it must be one of chat, messages, responses", access.Name, access.Spec.Protocol)
		}
	}
	return nil
}

// validateRouteOffers enforces the per-offer grammar the CEL rule expresses: a
// served id, when written, is an RFC-1123 name.
func validateRouteOffers(name string, offers []v1alpha1.RouteOffer) error {
	for i, offer := range offers {
		if offer.Serving == "" {
			return fmt.Errorf("ModelRoute %q offer %d declares no serving", name, i)
		}
		if offer.ID != "" && !routeOfferIDPattern.MatchString(offer.ID) {
			return fmt.Errorf("ModelRoute %q offer %q declares id %q; a served id must be an RFC-1123 name", name, offer.Serving, offer.ID)
		}
	}
	return nil
}

// validateRouteTransport enforces the transport rules the CRD states for a
// route: a shaper dials an absolute upstream URL through a known auth mode, and
// the route publishes surfaces it can actually reach. The shape checks live in
// validateRouteShape; this function covers the parts that parse or enumerate.
func validateRouteTransport(name string, spec *v1alpha1.ModelRouteSpec) error {
	if spec.Transport.Kind == v1alpha1.TransportShaper {
		if err := validateAbsoluteURL(name, "transport.upstream", spec.Transport.Upstream); err != nil {
			return err
		}
	}
	if spec.Transport.Shaper != nil {
		mode := spec.Transport.Shaper.AuthMode
		if mode != "" && !credentialMode(mode) && mode != "none" && mode != "auto" {
			return fmt.Errorf("ModelRoute %q declares unknown shaper auth_mode %q; it must be auto, none, bearer, api-key, x-api-key, awsSigV4, or header:NAME", name, mode)
		}
	}
	return nil
}

// validateAbsoluteURL parses a field expected to hold an absolute URL.
func validateAbsoluteURL(name, field, value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("ModelRoute %q %s %q: %w", name, field, value, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("ModelRoute %q %s %q is not an absolute URL", name, field, value)
	}
	return nil
}
