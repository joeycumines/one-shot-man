package userk8s

import (
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

// ResolvedOffer is one route offer with its effective capability and the
// serving/model it resolves to.
type ResolvedOffer struct {
	ID        string
	Serving   *v1alpha1.ModelServing
	Model     *v1alpha1.Model
	Effective EffectiveModel
}

// validateServings checks provider/model references, per-provider defaults,
// and the narrowing rule. The narrowing check is here (not only in CEL)
// because the files backend never runs an API server.
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
		if serving.Spec.Default {
			key := registryName(provider)
			if existing, ok := defaults[key]; ok {
				return fmt.Errorf("ModelServing %q and %q both declare default for provider %q", existing, serving.Name, serving.Spec.Provider)
			}
			defaults[key] = serving.Name
		}
		if _, err := effectiveModel(model, serving); err != nil {
			return err
		}
	}
	return nil
}

// validateSelections checks candidate references, offer references, and cycles.
func (o *Objects) validateSelections() error {
	for i := range o.Selections {
		selection := &o.Selections[i]
		if len(selection.Spec.Candidates) == 0 {
			return fmt.Errorf("ModelSelection %q declares no candidates", selection.Name)
		}
		for j := range selection.Spec.Candidates {
			candidate := &selection.Spec.Candidates[j]
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

// routeOffers resolves the effective inventory of one route. An empty Offers
// list on a shaper/direct route means every non-deprecated serving of the
// access's provider; a remote route must declare its inventory.
func (o *Objects) routeOffers(route *v1alpha1.ModelRoute) ([]ResolvedOffer, error) {
	spec := &route.Spec
	declared := make([]v1alpha1.RouteOffer, 0, len(spec.Offers))
	if len(spec.Offers) > 0 {
		declared = append(declared, spec.Offers...)
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
		for i := range o.Servings {
			serving := &o.Servings[i]
			if serving.Spec.Provider == access.Spec.Provider && !serving.Spec.Deprecated {
				declared = append(declared, v1alpha1.RouteOffer{Serving: serving.Name})
			}
		}
		sort.Slice(declared, func(i, j int) bool { return declared[i].Serving < declared[j].Serving })
	}
	out := make([]ResolvedOffer, 0, len(declared))
	seen := map[string]bool{}
	for _, entry := range declared {
		serving, ok := o.lookupServing(entry.Serving)
		if !ok {
			return nil, fmt.Errorf("ModelRoute %q references undeclared ModelServing %q", route.Name, entry.Serving)
		}
		model, ok := o.lookupModel(serving.Spec.Model)
		if !ok {
			return nil, fmt.Errorf("ModelServing %q references undeclared Model %q", serving.Name, serving.Spec.Model)
		}
		if serving.Spec.Deprecated || model.Spec.Deprecated {
			continue
		}
		if spec.Access != "" {
			access, ok := o.lookupAccess(spec.Access)
			if !ok {
				return nil, fmt.Errorf("ModelRoute %q references undeclared ModelAccess %q", route.Name, spec.Access)
			}
			if serving.Spec.Provider != access.Spec.Provider {
				return nil, fmt.Errorf("ModelRoute %q access %q serves provider %q but offering %q belongs to provider %q", route.Name, access.Name, access.Spec.Provider, serving.Name, serving.Spec.Provider)
			}
		}
		effective, err := effectiveModel(model, serving)
		if err != nil {
			return nil, err
		}
		id := entry.ID
		if id == "" {
			id = serving.Name
		}
		if seen[id] {
			return nil, fmt.Errorf("ModelRoute %q exposes served id %q more than once", route.Name, id)
		}
		seen[id] = true
		out = append(out, ResolvedOffer{ID: id, Serving: serving, Model: model, Effective: effective})
	}
	return out, nil
}

// validateRoutes performs the structural and cross-resource checks a files
// artifact never has an API server evaluate.
func (o *Objects) validateRoutes() error {
	shaperAccesses := map[string]string{}
	for i := range o.Routes {
		route := &o.Routes[i]
		if err := o.validateRoute(route, shaperAccesses); err != nil {
			return err
		}
	}
	return nil
}

func (o *Objects) validateRoute(route *v1alpha1.ModelRoute, shaperAccesses map[string]string) error {
	name := route.Name
	if name == "" {
		return fmt.Errorf("ModelRoute with empty metadata.name")
	}
	spec := &route.Spec
	if err := validateRouteShape(name, spec); err != nil {
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
	// A shaper auth mode that presents a credential is a promise that a
	// credential exists. With no binding (or a binding whose scheme is none)
	// there is nothing to present, and the shaper would either send no auth to
	// an upstream that rejects it or fail at its first request. Refuse the
	// catalog instead of starting a mount that cannot honour its declaration.
	if spec.Transport.Kind == v1alpha1.TransportShaper && spec.Transport.Shaper != nil {
		credentialless := spec.Credential == "" || (binding != nil && binding.Spec.Scheme == v1alpha1.CredentialNone)
		if mode := strings.TrimSpace(spec.Transport.Shaper.AuthMode); credentialless && credentialMode(mode) {
			return fmt.Errorf("ModelRoute %q declares shaper auth mode %q but names no credential; a credential mode needs a LocalSecretBinding", name, mode)
		}
	}
	offers, err := o.routeOffers(route)
	if err != nil {
		return err
	}
	if len(offers) == 0 {
		// A route whose whole inventory is retired is a catalog statement, not
		// a defect: the gateway filters deprecated offers and never spawns a
		// section for it. It must not claim the access and block a live route
		// for the same access.
		return nil
	}
	if spec.Transport.Kind == v1alpha1.TransportShaper {
		if previous, ok := shaperAccesses[access.Name]; ok {
			return fmt.Errorf("ModelRoutes %q and %q both mount access %q with a shaper; a shaper suite serves one section per route, so one access may be materialized by only one route", previous, name, access.Name)
		}
		shaperAccesses[access.Name] = name
	}
	return nil
}

func validateRouteShape(name string, spec *v1alpha1.ModelRouteSpec) error {
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

// credentialMode reports whether a shaper --auth-mode value presents a
// credential. auto and none do not: auto lets the shaper choose from what it
// was given, and none strips credentials. The header:NAME form does.
func credentialMode(mode string) bool {
	switch mode {
	case "", "none", "auto":
		return false
	case "bearer", "api-key", "x-api-key", "awsSigV4":
		return true
	}
	return strings.HasPrefix(mode, "header:")
}

// validateSurfaceSupport enforces the transport's real capability. A declared
// surface is a promise; advertising one the protocol cannot reach sends clients
// to a 404.
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

// validateShaperMount requires every endpoint of a shaper route to resolve to
// one mount prefix. The shaper strips the prefix before dispatch, so endpoints
// that disagree would make some declared surface unreachable.
func validateShaperMount(name string, spec *v1alpha1.ModelRouteSpec) error {
	var prefix string
	for _, surface := range spec.Surfaces {
		path, err := endpointPath(string(spec.Endpoints[surface]))
		if err != nil {
			return fmt.Errorf("ModelRoute %q endpoint for %q: %w", name, surface, err)
		}
		trimmed := strings.TrimSuffix(strings.TrimRight(path, "/"), "/v1")
		if prefix == "" {
			prefix = trimmed
			continue
		}
		if trimmed != prefix {
			return fmt.Errorf("ModelRoute %q endpoints disagree on the mount prefix (%q vs %q)", name, prefix, trimmed)
		}
	}
	if prefix == "" || prefix == "/" {
		return fmt.Errorf("ModelRoute %q shaper endpoints carry no mount prefix", name)
	}
	return nil
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
