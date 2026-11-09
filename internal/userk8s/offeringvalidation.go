package userk8s

import (
	"fmt"
	"net/url"
	pathpkg "path"
	"slices"
	"sort"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

// ResolvedOffer is a route offer with its effective capability.
type ResolvedOffer struct {
	ID        string
	Serving   *v1alpha1.ModelServing
	Model     *v1alpha1.Model
	Effective EffectiveModel
}

// validateServings checks provider/model refs, defaults, and narrowing.
func (o *Objects) validateServings() error {
	defaults := map[string]string{}
	for i := range o.Servings {
		serving := &o.Servings[i]
		provider, ok := o.lookupProvider(serving.Spec.Provider)
		if !ok {
			return fmt.Errorf("ModelServing %q references undeclared ModelProvider %q", serving.Name, serving.Spec.Provider)
		}
		model, ok := o.lookupModel(serving.Spec.Model)
		if !ok {
			return fmt.Errorf("ModelServing %q references undeclared Model %q", serving.Name, serving.Spec.Model)
		}
		if serving.Spec.ProviderModelID == "" {
			return fmt.Errorf("ModelServing %q declares no provider_model_id", serving.Name)
		}
		// Priority defaults to 100; zero is unset and rejected when explicit.
		if serving.Spec.Priority <= 0 {
			return fmt.Errorf("ModelServing %q declares priority %d; it must be at least 1 (omit it to take the default 100)", serving.Name, serving.Spec.Priority)
		}
		if serving.Spec.Default {
			key := registryName(provider)
			if existing, ok := defaults[key]; ok {
				return fmt.Errorf("ModelServing %q and %q both declare default for provider %q", existing, serving.Name, serving.Spec.Provider)
			}
			defaults[key] = serving.Name
		}
		effective, err := effectiveModel(model, serving)
		if err != nil {
			return err
		}
		// Tool settings are authored against the canonical Model but run on
		// narrowed capabilities; reject contradictions here.
		if err := validateToolSettingsAgainstEffective(model, serving, effective); err != nil {
			return err
		}
	}
	return nil
}

// validateSelections checks candidate/offer refs and cycles.
func (o *Objects) validateSelections() error {
	for i := range o.Selections {
		selection := &o.Selections[i]
		if selection.Spec.DisplayName == "" {
			return fmt.Errorf("ModelSelection %q declares no display_name", selection.Name)
		}
		switch selection.Spec.Strategy {
		case v1alpha1.SelectionOrdered, v1alpha1.SelectionPriority, v1alpha1.SelectionDynamic:
		default:
			return fmt.Errorf("ModelSelection %q declares unknown strategy %q; it must be ordered, priority, or dynamic", selection.Name, selection.Spec.Strategy)
		}
		if len(selection.Spec.Candidates) == 0 {
			return fmt.Errorf("ModelSelection %q declares no candidates", selection.Name)
		}
		for j := range selection.Spec.Candidates {
			candidate := &selection.Spec.Candidates[j]
			// The CRD requires exactly one reference. A switch would silently
			// honour the first field and ignore the rest, letting a files
			// artifact carry a candidate the cluster would reject.
			references := 0
			for _, set := range []bool{
				candidate.Serving != "",
				candidate.Model != "",
				candidate.Selection != "",
				candidate.Offer != nil,
			} {
				if set {
					references++
				}
			}
			if references != 1 {
				return fmt.Errorf("ModelSelection %q candidate %d sets %d references; exactly one of serving, model, selection, or offer is required", selection.Name, j, references)
			}
			switch {
			case candidate.Serving != "":
				if _, ok := o.lookupServing(candidate.Serving); !ok {
					return fmt.Errorf("ModelSelection %q candidate references undeclared ModelServing %q", selection.Name, candidate.Serving)
				}
			case candidate.Model != "":
				if _, ok := o.lookupModel(candidate.Model); !ok {
					return fmt.Errorf("ModelSelection %q candidate references undeclared Model %q", selection.Name, candidate.Model)
				}
			case candidate.Selection != "":
				if _, ok := o.lookupSelection(candidate.Selection); !ok {
					return fmt.Errorf("ModelSelection %q candidate references undeclared ModelSelection %q", selection.Name, candidate.Selection)
				}
			case candidate.Offer != nil:
				route, ok := o.lookupRoute(candidate.Offer.Route)
				if !ok {
					return fmt.Errorf("ModelSelection %q offer candidate references undeclared ModelRoute %q", selection.Name, candidate.Offer.Route)
				}
				offers, err := o.routeOffers(route)
				if err != nil {
					return err
				}
				found := false
				for _, offer := range offers {
					if offer.ID == candidate.Offer.ID {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("ModelSelection %q offer candidate references route %q id %q, which the route does not expose", selection.Name, candidate.Offer.Route, candidate.Offer.ID)
				}
			default:
				return fmt.Errorf("ModelSelection %q has a candidate with no reference", selection.Name)
			}
		}
	}
	for i := range o.Selections {
		if err := o.validateSelectionCycle(&o.Selections[i], nil); err != nil {
			return err
		}
	}
	return nil
}

func (o *Objects) validateSelectionCycle(selection *v1alpha1.ModelSelection, path []string) error {
	if slices.Contains(path, selection.Name) {
		return fmt.Errorf("ModelSelection %q participates in a candidate cycle: %s", selection.Name, strings.Join(append(path, selection.Name), " -> "))
	}
	next := append(slices.Clone(path), selection.Name)
	for i := range selection.Spec.Candidates {
		ref := selection.Spec.Candidates[i].Selection
		if ref == "" {
			continue
		}
		child, ok := o.lookupSelection(ref)
		if !ok {
			return fmt.Errorf("ModelSelection %q candidate references undeclared ModelSelection %q", selection.Name, ref)
		}
		if err := o.validateSelectionCycle(child, next); err != nil {
			return err
		}
	}
	return nil
}

// routeOffers resolves the effective inventory of a route.
func (o *Objects) routeOffers(route *v1alpha1.ModelRoute) ([]ResolvedOffer, error) {
	spec := &route.Spec
	explicit := len(spec.Offers) > 0
	type routeCandidate struct {
		offer   v1alpha1.RouteOffer
		serving *v1alpha1.ModelServing
	}
	var candidates []routeCandidate
	if explicit {
		candidates = make([]routeCandidate, 0, len(spec.Offers))
		for _, offer := range spec.Offers {
			candidates = append(candidates, routeCandidate{offer: offer})
		}
	} else {
		if spec.Transport.Kind == v1alpha1.TransportRemote {
			return nil, fmt.Errorf("remote ModelRoute %q must declare its offers inventory", route.Name)
		}
		if spec.Access == "" {
			return nil, fmt.Errorf("ModelRoute %q exposes no offers and names no access to default from", route.Name)
		}
		access, ok := o.lookupAccess(spec.Access)
		if !ok {
			return nil, fmt.Errorf("ModelRoute %q references undeclared ModelAccess %q", route.Name, spec.Access)
		}
		accessProvider, ok := o.lookupProvider(access.Spec.Provider)
		if !ok {
			return nil, fmt.Errorf("ModelAccess %q references undeclared ModelProvider %q", access.Name, access.Spec.Provider)
		}
		candidates = make([]routeCandidate, 0, len(o.Servings))
		for i := range o.Servings {
			serving := &o.Servings[i]
			provider, ok := o.lookupProvider(serving.Spec.Provider)
			if !ok || provider != accessProvider || serving.Spec.Deprecated {
				continue
			}
			candidates = append(candidates, routeCandidate{
				offer:   v1alpha1.RouteOffer{Serving: serving.Name},
				serving: serving,
			})
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].offer.Serving < candidates[j].offer.Serving })
	}
	out := make([]ResolvedOffer, 0, len(candidates))
	seen := map[string]bool{}
	var access *v1alpha1.ModelAccess
	if spec.Access != "" {
		var ok bool
		access, ok = o.lookupAccess(spec.Access)
		if !ok {
			return nil, fmt.Errorf("ModelRoute %q references undeclared ModelAccess %q", route.Name, spec.Access)
		}
	}
	for _, entry := range candidates {
		serving := entry.serving
		if serving == nil {
			var ok bool
			serving, ok = o.lookupServing(entry.offer.Serving)
			if !ok {
				return nil, fmt.Errorf("ModelRoute %q references undeclared ModelServing %q", route.Name, entry.offer.Serving)
			}
		}
		model, ok := o.lookupModel(serving.Spec.Model)
		if !ok {
			return nil, fmt.Errorf("ModelServing %q references undeclared Model %q", serving.Name, serving.Spec.Model)
		}
		// Providers are compared as resolved objects, not raw strings.
		if access != nil {
			accessProvider, ok := o.lookupProvider(access.Spec.Provider)
			if !ok {
				return nil, fmt.Errorf("ModelAccess %q references undeclared ModelProvider %q", access.Name, access.Spec.Provider)
			}
			servingProvider, ok := o.lookupProvider(serving.Spec.Provider)
			if !ok {
				return nil, fmt.Errorf("ModelServing %q references undeclared ModelProvider %q", serving.Name, serving.Spec.Provider)
			}
			if servingProvider != accessProvider {
				return nil, fmt.Errorf("ModelRoute %q access %q serves provider %q but offering %q belongs to provider %q", route.Name, access.Name, access.Spec.Provider, serving.Name, serving.Spec.Provider)
			}
		}
		id := entry.offer.ID
		if id == "" {
			id = serving.Name
		}
		if seen[id] {
			return nil, fmt.Errorf("ModelRoute %q exposes served id %q more than once", route.Name, id)
		}
		seen[id] = true
		// Only a defaulted inventory is filtered; an explicit offer is a pin.
		if !explicit && (serving.Spec.Deprecated || model.Spec.Deprecated) {
			continue
		}
		effective, err := effectiveModel(model, serving)
		if err != nil {
			return nil, err
		}
		out = append(out, ResolvedOffer{ID: id, Serving: serving, Model: model, Effective: effective})
	}
	return out, nil
}

// validateRoutes performs structural and cross-resource checks.
func (o *Objects) validateRoutes() error {
	// Two shaper routes may share one ModelAccess but not a mount prefix.
	mounts := map[string]string{}
	for i := range o.Routes {
		route := &o.Routes[i]
		if err := o.validateRoute(route); err != nil {
			return err
		}
		if route.Spec.Transport.Kind != v1alpha1.TransportShaper {
			continue
		}
		// A fully retired route claims no mount path.
		offers, err := o.routeOffers(route)
		if err != nil {
			return err
		}
		if len(offers) == 0 {
			continue
		}
		prefix, err := shaperMountPrefix(route.Name, &route.Spec)
		if err != nil {
			return err
		}
		if previous, ok := mounts[prefix]; ok {
			return fmt.Errorf("ModelRoutes %q and %q both mount the shaper at prefix %q; two shaper sections cannot claim the same local path", previous, route.Name, prefix)
		}
		mounts[prefix] = route.Name
	}
	return nil
}

// shaperMountPrefix returns the mount prefix for a shaper route.
func shaperMountPrefix(name string, spec *v1alpha1.ModelRouteSpec) (string, error) {
	var prefix string
	for i, surface := range spec.Surfaces {
		path, err := endpointPath(string(spec.Endpoints[surface]))
		if err != nil {
			return "", fmt.Errorf("ModelRoute %q endpoint for %q: %w", name, surface, err)
		}
		trimmed := cleanEndpointPath(path)
		switch surface {
		case "chat", "responses":
			if rest, ok := strings.CutSuffix(trimmed, "/v1"); ok {
				trimmed = strings.TrimSuffix(rest, "/openai")
			} else if trimmed == "/openai" {
				trimmed = ""
			}
		case "anthropic":
			trimmed = strings.TrimSuffix(trimmed, "/v1")
			trimmed = strings.TrimSuffix(trimmed, "/anthropic")
		}
		trimmed = strings.TrimRight(trimmed, "/")
		if trimmed == "" {
			return "", fmt.Errorf("ModelRoute %q shaper endpoint for surface %q carries no mount prefix (path %q)", name, surface, path)
		}
		if i == 0 {
			prefix = trimmed
			continue
		}
		if trimmed != prefix {
			return "", fmt.Errorf("ModelRoute %q endpoints disagree on the mount prefix (%q vs %q)", name, prefix, trimmed)
		}
	}
	if prefix == "" {
		return "", fmt.Errorf("ModelRoute %q shaper endpoints carry no mount prefix", name)
	}
	return prefix, nil
}

// cleanEndpointPath normalizes an endpoint path like the router.
func cleanEndpointPath(path string) string {
	return strings.TrimRight(pathpkg.Clean(path), "/")
}

func (o *Objects) validateRoute(route *v1alpha1.ModelRoute) error {
	name := route.Name
	if name == "" {
		return fmt.Errorf("ModelRoute with empty metadata.name")
	}
	spec := &route.Spec
	if err := validateRouteShape(name, spec); err != nil {
		return err
	}
	if err := validateRouteOffers(name, spec.Offers); err != nil {
		return err
	}
	if err := validateRouteTransport(name, spec); err != nil {
		return err
	}
	var access *v1alpha1.ModelAccess
	if spec.Access == "" {
		if spec.Transport.Kind != v1alpha1.TransportRemote {
			return fmt.Errorf("ModelRoute %q is %s but names no access", name, spec.Transport.Kind)
		}
	} else {
		var ok bool
		access, ok = o.lookupAccess(spec.Access)
		if !ok {
			return fmt.Errorf("ModelRoute %q references undeclared ModelAccess %q", name, spec.Access)
		}
		if spec.Transport.Kind == v1alpha1.TransportShaper {
			if err := validateShaperMount(name, spec); err != nil {
				return err
			}
		}
		if spec.Transport.Kind != v1alpha1.TransportRemote {
			if err := validateSurfaceSupport(name, access.Spec.Protocol, spec); err != nil {
				return err
			}
		}
	}
	var binding *v1alpha1.LocalSecretBinding
	if spec.Credential != "" {
		resolved, ok := o.lookupBinding(spec.Credential)
		if !ok {
			return fmt.Errorf("ModelRoute %q references undeclared LocalSecretBinding %q", name, spec.Credential)
		}
		binding = resolved
		if access, ok := o.lookupAccess(spec.Access); ok {
			matched, err := selectorMatches(&binding.Spec.Selector, accessLabelSet(access))
			if err != nil {
				return fmt.Errorf("ModelRoute %q credential %q: %w", name, spec.Credential, err)
			}
			if !matched {
				return fmt.Errorf("ModelRoute %q credential %q does not select ModelAccess %q; the binding selector must match the access labels", name, spec.Credential, access.Name)
			}
		}
	}
	// A shaper auth mode presenting a credential needs a binding.
	if spec.Transport.Kind == v1alpha1.TransportShaper && spec.Transport.Shaper != nil {
		credentialless := spec.Credential == "" || (binding != nil && binding.Spec.EffectiveScheme() == v1alpha1.CredentialNone)
		if mode := strings.TrimSpace(spec.Transport.Shaper.AuthMode); credentialless && credentialMode(mode) {
			return fmt.Errorf("ModelRoute %q declares shaper auth mode %q but names no credential; a credential mode needs a LocalSecretBinding", name, mode)
		}
	}
	offers, err := o.routeOffers(route)
	if err != nil {
		return err
	}
	if len(offers) == 0 {
		return nil
	}
	return nil
}

func validateRouteShape(name string, spec *v1alpha1.ModelRouteSpec) error {
	// Order defaults to 100; zero is unset and rejected when explicit.
	if spec.Order <= 0 {
		return fmt.Errorf("ModelRoute %q declares order %d; it must be at least 1 (omit it to take the default 100)", name, spec.Order)
	}
	switch spec.Naming {
	case v1alpha1.RouteNamingServed, v1alpha1.RouteNamingWire, v1alpha1.RouteNamingRegistry, v1alpha1.RouteNamingCanonical:
	default:
		return fmt.Errorf("ModelRoute %q declares unknown naming %q", name, spec.Naming)
	}
	if len(spec.Surfaces) == 0 {
		return fmt.Errorf("ModelRoute %q declares no surfaces", name)
	}
	seen := map[string]bool{}
	for _, surface := range spec.Surfaces {
		switch surface {
		case "anthropic", "chat", "responses":
		default:
			return fmt.Errorf("ModelRoute %q declares unknown surface %q", name, surface)
		}
		if seen[surface] {
			return fmt.Errorf("ModelRoute %q declares surface %q twice", name, surface)
		}
		seen[surface] = true
	}
	if len(spec.Endpoints) != len(spec.Surfaces) {
		return fmt.Errorf("ModelRoute %q declares %d surfaces but %d endpoints", name, len(spec.Surfaces), len(spec.Endpoints))
	}
	for _, surface := range spec.Surfaces {
		endpoint, ok := spec.Endpoints[surface]
		if !ok || strings.TrimSpace(string(endpoint)) == "" {
			return fmt.Errorf("ModelRoute %q declares surface %q with no endpoint", name, surface)
		}
	}
	for surface := range spec.Endpoints {
		if !seen[surface] {
			return fmt.Errorf("ModelRoute %q declares an endpoint for undeclared surface %q", name, surface)
		}
	}
	switch spec.Transport.Kind {
	case v1alpha1.TransportDirect:
		if spec.Naming != v1alpha1.RouteNamingWire {
			return fmt.Errorf("ModelRoute %q is direct but declares naming %q; a passthrough must use wire", name, spec.Naming)
		}
		if spec.Transport.Shaper != nil {
			return fmt.Errorf("ModelRoute %q is direct but carries shaper configuration", name)
		}
	case v1alpha1.TransportShaper:
		if strings.TrimSpace(spec.Transport.Upstream) == "" {
			return fmt.Errorf("ModelRoute %q is a shaper route but declares no upstream", name)
		}
		if spec.Naming != v1alpha1.RouteNamingServed {
			return fmt.Errorf("ModelRoute %q is a shaper route but declares naming %q; the shaper model table publishes served ids", name, spec.Naming)
		}
	case v1alpha1.TransportRemote:
		if spec.Transport.Shaper != nil {
			return fmt.Errorf("ModelRoute %q is remote but carries shaper configuration", name)
		}
	case "":
		return fmt.Errorf("ModelRoute %q declares no transport", name)
	default:
		return fmt.Errorf("ModelRoute %q declares unknown transport %q", name, spec.Transport.Kind)
	}
	return nil
}

// credentialMode reports whether an auth mode presents a credential.
func credentialMode(mode string) bool {
	switch mode {
	case "", "none", "auto":
		return false
	case "bearer", "api-key", "x-api-key", "awsSigV4":
		return true
	}
	return strings.HasPrefix(mode, "header:")
}

// validateSurfaceSupport enforces transport capability.
func validateSurfaceSupport(name string, protocol v1alpha1.WireProtocol, spec *v1alpha1.ModelRouteSpec) error {
	supported := map[string]bool{}
	switch protocol {
	case v1alpha1.WireProtocolChat:
		supported["chat"] = true
		supported["anthropic"] = true
		supported["responses"] = true
	case v1alpha1.WireProtocolMessages:
		supported["anthropic"] = true
	case v1alpha1.WireProtocolResponses:
		supported["responses"] = true
		supported["anthropic"] = true
	default:
		return fmt.Errorf("ModelRoute %q access declares unknown protocol %q", name, protocol)
	}
	if spec.Transport.Kind == v1alpha1.TransportDirect {
		direct := map[v1alpha1.WireProtocol]string{
			v1alpha1.WireProtocolChat:      "chat",
			v1alpha1.WireProtocolMessages:  "anthropic",
			v1alpha1.WireProtocolResponses: "responses",
		}
		surface, ok := direct[protocol]
		if !ok {
			return fmt.Errorf("ModelRoute %q direct transport has unknown protocol %q", name, protocol)
		}
		supported = map[string]bool{surface: true}
	}
	for _, surface := range spec.Surfaces {
		if !supported[surface] {
			return fmt.Errorf("ModelRoute %q publishes surface %q, which protocol %q cannot serve on a %s transport", name, surface, protocol, spec.Transport.Kind)
		}
	}
	return nil
}

// validateShaperMount requires a non-empty mount prefix.
func validateShaperMount(name string, spec *v1alpha1.ModelRouteSpec) error {
	_, err := shaperMountPrefix(name, spec)
	return err
}

func endpointPath(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%q is not an absolute URL", endpoint)
	}
	if parsed.Path == "" {
		return "", fmt.Errorf("%q has no path", endpoint)
	}
	return parsed.Path, nil
}
