package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SurfaceEndpoint is the base URL a client resolves one wire surface against.
//
// +kubebuilder:validation:MinLength=1
type SurfaceEndpoint string

// ModelRouteNaming names how a client addresses a model on a route.
type ModelRouteNaming string

const (
	// RouteNamingServed means the client sends the route offer's served id
	// (the shaper surrogate) and the transport rewrites it to the serving's
	// wire id.
	RouteNamingServed ModelRouteNaming = "served"
	// RouteNamingWire means the client sends the serving's
	// spec.provider_model_id and the transport forwards it verbatim.
	RouteNamingWire ModelRouteNaming = "wire"
	// RouteNamingRegistry means the client sends the serving's catalog
	// registry key, resolved by the served service.
	RouteNamingRegistry ModelRouteNaming = "registry"
	// RouteNamingCanonical means the client sends the Model's canonical id,
	// resolved by the served service.
	RouteNamingCanonical ModelRouteNaming = "canonical"
)

// ModelRouteTransportKind names how a route reaches its access.
type ModelRouteTransportKind string

const (
	// TransportDirect means the client speaks to the provider endpoint
	// directly. No local process rewrites anything: the route publishes only
	// the access protocol's own surface and uses wire naming.
	TransportDirect ModelRouteTransportKind = "direct"
	// TransportShaper means a local ai-concurrency-shaper process fronts the
	// access. It rewrites served ids and synthesises the client surfaces the
	// protocol can reach.
	TransportShaper ModelRouteTransportKind = "shaper"
	// TransportRemote means an external service publishes the endpoints. The
	// route is an inventory: it trusts the service's declared surfaces and
	// naming, and resolves no local credentials.
	TransportRemote ModelRouteTransportKind = "remote"
)

// RouteOffer binds one ModelServing to the id this route exposes it under.
//
// +kubebuilder:validation:XValidation:rule="!has(self.id) || size(self.id) == 0 || self.id.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')",message="offer.id must be an RFC-1123 name when set"
type RouteOffer struct {
	// Serving is a ModelServing reference (metadata name or registry key).
	// +kubebuilder:validation:MinLength=1
	Serving string `json:"serving"`

	// ID is the served id; defaults to the serving's metadata.name.
	// +optional
	ID string `json:"id,omitempty"`
}

// ModelRoute is one entity's access item: the provider access, the entity-held
// credential, the connection means, and the serving inventory exposed under
// this item. It is the tool-visible grouping level above model variants:
// credentials attach here, not to a Model.
//
// metadata.name is also the shaper section identity for a shaper transport, so
// several routes (accounts, regions, protocols) coexist in one suite.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:validation:XValidation:rule="self.metadata.name.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')",message="metadata.name must match ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
type ModelRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ModelRouteSpec `json:"spec,omitempty"`
}

// ModelRouteSpec is one entity's exposure.
//
// +kubebuilder:validation:XValidation:rule="self.surfaces.size() > 0",message="spec.surfaces must declare at least one client surface"
// +kubebuilder:validation:XValidation:rule="self.surfaces.all(s, s in ['anthropic', 'chat', 'responses', 'gemini'])",message="spec.surfaces items must be one of anthropic|chat|responses|gemini"
// +kubebuilder:validation:XValidation:rule="self.endpoints.size() == self.surfaces.size() && self.surfaces.all(s, s in self.endpoints) && self.endpoints.all(k, k in self.surfaces)",message="spec.endpoints must declare exactly the surfaces in spec.surfaces"
// +kubebuilder:validation:XValidation:rule="self.endpoints.all(k, v, v.size() > 0)",message="spec.endpoints values must be non-empty base URLs"
// +kubebuilder:validation:XValidation:rule="self.transport.kind != 'shaper' || has(self.access)",message="shaper transport requires spec.access"
// +kubebuilder:validation:XValidation:rule="self.transport.kind != 'direct' || has(self.access)",message="direct transport requires spec.access"
// +kubebuilder:validation:XValidation:rule="self.transport.kind != 'shaper' || self.naming == 'served'",message="shaper transport requires naming served"
// +kubebuilder:validation:XValidation:rule="self.transport.kind != 'direct' || self.naming == 'wire'",message="direct transport requires naming wire"
// +kubebuilder:validation:XValidation:rule="self.transport.kind != 'shaper' || has(self.transport.upstream)",message="shaper transport requires spec.transport.upstream"
// +kubebuilder:validation:XValidation:rule="self.transport.kind == 'shaper' || !has(self.transport.shaper)",message="spec.transport.shaper is only valid for shaper transport"
// +kubebuilder:validation:XValidation:rule="self.transport.kind == 'shaper' || self.transport.kind == 'direct' || (has(self.offers) && self.offers.size() > 0)",message="a remote route must declare its offers inventory"
type ModelRouteSpec struct {
	// Access names the logical ModelAccess this route dials. Required for
	// shaper and direct transports; absent for a remote inventory.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Access string `json:"access,omitempty"`

	// Credential names the LocalSecretBinding (same namespace) that supplies
	// the entity's credential for this access item. Optional for
	// credential-less accesses or remote transports.
	// +optional
	Credential string `json:"credential,omitempty"`

	// Offers is the serving inventory exposed under this access item. Empty on
	// a shaper/direct route means every serving of the access's provider.
	// +optional
	Offers []RouteOffer `json:"offers,omitempty"`

	// Surfaces lists the client wire surfaces this route publishes. "gemini"
	// is the Google GenerateContent client surface (Antigravity CLI); a shaper
	// transport serves it by transcoding the request to the access protocol.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:items:Enum=anthropic;chat;responses;gemini
	Surfaces []string `json:"surfaces"`

	// Endpoints maps each served surface to the client base URL.
	Endpoints map[string]SurfaceEndpoint `json:"endpoints"`

	// Naming declares how a client addresses a model on this route.
	// +kubebuilder:validation:Enum=served;wire;registry;canonical
	Naming ModelRouteNaming `json:"naming"`

	// Order is the preference among routes that expose the same offering;
	// lower is more preferred. Zero is the unset value and is rejected when
	// declared explicitly: the default replaces it with 100.
	// +kubebuilder:default=100
	// +kubebuilder:validation:Minimum=1
	// +optional
	Order int32 `json:"order,omitempty"`

	// Transport names how this route reaches the access/service.
	Transport ModelRouteTransport `json:"transport"`
}

// EffectiveOrder is the route's preference with the CRD default applied for
// consumers that read objects from a files artifact, where no API server
// defaults the field.
func (spec ModelRouteSpec) EffectiveOrder() int32 {
	if spec.Order == 0 {
		return 100
	}
	return spec.Order
}

// ModelRouteTransport describes the mechanism behind a route.
type ModelRouteTransport struct {
	// Kind is direct, shaper, or remote.
	// +kubebuilder:validation:Enum=direct;shaper;remote
	Kind ModelRouteTransportKind `json:"kind"`

	// Upstream is the provider endpoint a shaper transport dials.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Upstream string `json:"upstream,omitempty"`

	// Shaper carries the operating envelope for a shaper transport.
	// +optional
	Shaper *ShaperTransportConfig `json:"shaper,omitempty"`
}

// ShaperTransportConfig carries the operational parameters of one shaper
// transport. All fields are optional; absent fields let the shaper binary's
// defaults apply.
type ShaperTransportConfig struct {
	// AuthMode is the upstream auth mode: auto, none, bearer, x-api-key,
	// api-key, header:NAME.
	// +optional
	// +kubebuilder:validation:MinLength=1
	AuthMode string `json:"auth_mode,omitempty"`

	// Concurrency is the max concurrent limited requests (-concurrency).
	// +kubebuilder:validation:Minimum=1
	// +optional
	Concurrency *int32 `json:"concurrency,omitempty"`

	// LimitAll enables limiting all requests, not just matching routes (-limit-all).
	// +optional
	LimitAll *bool `json:"limit_all,omitempty"`

	// QueueTimeout is the max time a request waits in the queue (-queue-timeout).
	// +optional
	QueueTimeout string `json:"queue_timeout,omitempty"`

	// Retry is the max retries for limited requests (-retry).
	// +optional
	Retry *int32 `json:"retry,omitempty"`

	// RetryMinDelay is the minimum delay before retrying (-retry-min-delay).
	// +optional
	RetryMinDelay string `json:"retry_min_delay,omitempty"`

	// RetrySkip429 skips retrying 429 responses (-retry-skip-429).
	// +optional
	RetrySkip429 *bool `json:"retry_skip_429,omitempty"`

	// ReleaseCooldown is the delay after slot release before re-admission (-release-cooldown).
	// +optional
	ReleaseCooldown string `json:"release_cooldown,omitempty"`

	// CancelCooldown is the slot hold after client cancel (-cancel-cooldown).
	// +optional
	CancelCooldown string `json:"cancel_cooldown,omitempty"`

	// CircuitBreaker enables the circuit breaker (-circuit-breaker).
	// +optional
	CircuitBreaker *bool `json:"circuit_breaker,omitempty"`

	// CBThreshold is failures within window to trip circuit breaker (-cb-threshold).
	// +kubebuilder:validation:Minimum=1
	// +optional
	CBThreshold *int32 `json:"cb_threshold,omitempty"`

	// CBWindow is the failure counting window (-cb-window).
	// +optional
	CBWindow string `json:"cb_window,omitempty"`

	// CBPenalty is the base phantom concurrency hold time (-cb-penalty).
	// +optional
	CBPenalty string `json:"cb_penalty,omitempty"`

	// CBMaxPenalty is the max phantom concurrency hold time (-cb-max-penalty).
	// +optional
	CBMaxPenalty string `json:"cb_max_penalty,omitempty"`
}

// ModelRouteList is the list form of ModelRoute.
//
// +kubebuilder:object:root=true
type ModelRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ModelRoute `json:"items"`
}
