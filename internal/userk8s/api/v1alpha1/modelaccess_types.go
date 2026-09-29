package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AuthScheme names how a ModelAccess authenticates against its provider.
type AuthScheme string

const (
	// AuthSchemeNone means the access is usable without credentials.
	AuthSchemeNone AuthScheme = "none"
	// AuthSchemeBearer means the access expects a bearer credential supplied
	// through one environment variable per requiredEnv entry.
	AuthSchemeBearer AuthScheme = "bearer"
	// AuthSchemeAWSSigV4 means the access expects an AWS credential set
	// (access key id plus secret access key) supplied through requiredEnv.
	AuthSchemeAWSSigV4 AuthScheme = "awsSigV4"
	// AuthSchemeToolManaged means the invoked tool owns authentication and
	// the catalog only models the fact.
	AuthSchemeToolManaged AuthScheme = "toolManaged"
)

// ModelAccessAuth declares how an access authenticates: the scheme plus the
// environment variable names a selector-matched LocalSecretBinding must cover
// for resolution to complete.
type ModelAccessAuth struct {
	// Scheme is the authentication scheme of the access.
	// +kubebuilder:validation:Enum=none;bearer;awsSigV4;toolManaged
	Scheme AuthScheme `json:"scheme"`

	// RequiredEnv lists the environment variable names that must be resolved
	// before the access can launch. Coverage is checked by the engine against
	// the union of selector-matched LocalSecretBindings.
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:items:Pattern=`^[A-Za-z_][A-Za-z0-9_]*$`
	// +optional
	RequiredEnv []string `json:"requiredEnv,omitempty"`
}

// HeaderValue models an optional request header: an explicit value sent when
// force is true, and whether the client's default is acceptable.
type HeaderValue struct {
	// Value is the explicit header value used when Force is true.
	// +kubebuilder:default=""
	// +optional
	Value string `json:"value,omitempty"`

	// Auto is true when the client's default User-Agent is acceptable.
	// +kubebuilder:default=true
	// +optional
	Auto *bool `json:"auto,omitempty"`

	// Force is true when Value MUST be sent regardless of client defaults.
	// +kubebuilder:default=false
	// +optional
	Force bool `json:"force,omitempty"`
}

// SurfaceEndpoint is the base URL a client resolves one wire surface against.
//
// +kubebuilder:validation:MinLength=1
type SurfaceEndpoint string

// ModelAccess is the Identity analog: HOW to reach a provider — mode,
// user agent, anonymity, jurisdiction, auth — plus the concrete transport
// facts (endpoints per surface) and a preference order among accesses of the
// same provider (lower is more preferred). Cluster-scoped.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')",message="metadata.name must match ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ — emitted resource names are sanitized; the raw registry name lives in the one-shot-man/registry-name annotation"
type ModelAccess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ModelAccessSpec `json:"spec,omitempty"`
}

// ModelAccessSpec describes one way of reaching a ModelProvider.
//
// +kubebuilder:validation:XValidation:rule="!has(self.endpoints) || self.endpoints.size() > 0",message="spec.endpoints must declare at least one served surface"
// +kubebuilder:validation:XValidation:rule="!has(self.endpoints) || self.endpoints.all(k, v, k in ['responses', 'chat', 'anthropic'])",message="spec.endpoints keys must be a served surface (responses|chat|anthropic)"
// +kubebuilder:validation:XValidation:rule="!has(self.auth) || self.auth.scheme != 'bearer' || (has(self.auth.requiredEnv) && self.auth.requiredEnv.size() > 0)",message="auth scheme bearer requires at least one requiredEnv entry"
// +kubebuilder:validation:XValidation:rule="!has(self.auth) || self.auth.scheme != 'awsSigV4' || (has(self.auth.requiredEnv) && self.auth.requiredEnv.size() >= 2)",message="auth scheme awsSigV4 requires both credential entries in requiredEnv"
// +kubebuilder:validation:XValidation:rule="!has(self.auth) || self.auth.scheme != 'none' || !has(self.auth.requiredEnv) || self.auth.requiredEnv.size() == 0",message="auth scheme none must not declare requiredEnv"
type ModelAccessSpec struct {
	// Provider references a ModelProvider by name. Must resolve; the
	// cross-resource check is a controller-side gap, not expressible in CEL.
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`

	// Mode is the access-mode enum: direct reaches the provider's own
	// endpoint(s); shaper routes through a local protocol-transcoding shaper;
	// proxy reserves a future HTTP egress pathway.
	// +kubebuilder:validation:Enum=direct;shaper;proxy
	// +kubebuilder:default=direct
	// +optional
	Mode string `json:"mode,omitempty"`

	// UserAgent models the User-Agent header policy for this access.
	// +optional
	UserAgent *HeaderValue `json:"user_agent,omitempty"`

	// Anonymous is true when the access is usable without credentials.
	// +kubebuilder:default=false
	// +optional
	Anonymous bool `json:"anonymous,omitempty"`

	// Country is the informative ISO country code of the access pathway.
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=3
	// +optional
	Country string `json:"country,omitempty"`

	// Auth declares the authentication scheme and the environment variables
	// that must be covered by selector-matched LocalSecretBindings. Absent
	// means the access declares no credential needs.
	// +optional
	Auth *ModelAccessAuth `json:"auth,omitempty"`

	// Endpoints maps each served surface to the base URL a client resolves
	// that surface against. A surface absent here cannot be used.
	// +optional
	Endpoints map[string]SurfaceEndpoint `json:"endpoints,omitempty"`

	// Order is the preference among accesses of the same provider; lower is
	// more preferred.
	// +kubebuilder:default=100
	// +optional
	Order int32 `json:"order,omitempty"`

	// Deprecated marks an access no longer recommended for new tool
	// declarations.
	// +kubebuilder:default=false
	// +optional
	Deprecated bool `json:"deprecated,omitempty"`

	// Labels are Kubernetes-compatible labels, mirroring metadata.labels.
	// LocalSecretBinding selectors match against them.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// ModelAccessList is the list form of ModelAccess.
//
// +kubebuilder:object:root=true
type ModelAccessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ModelAccess `json:"items"`
}
