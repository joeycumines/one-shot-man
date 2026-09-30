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
// ## Spec-native configuration versus annotation-mapped configuration
//
// An access carries its configuration two ways, and the split is deliberate.
//
// A field in Spec is a fact THE CATALOG understands. The engine validates it
// (CEL on the CRD, plus the cross-resource checks in the loader), can
// cross-reference it against another field — auth_mode is only meaningful
// beside auth.requiredEnv — and can derive from it. ShaperConfig is the worked
// example: the gateway maps each field to exactly one ai-concurrency-shaper
// flag, refuses a shaper-mode access with no upstream, and validates the auth
// mode against the access's own credential slots. A wrong value is a registry
// defect the toolchain names, before anything is spawned.
//
// An annotation is a fact a CONSUMER understands and the CRD does not model.
// The motivating case is ai-concurrency-shaper
// (https://github.com/joeycumines/ai-concurrency-shaper), whose flag surface
// is large, vendor-owned, and moves independently of this API. Modelling its
// flags here would turn a catalog CRD into a mirror of an external binary's
// --help: every shaper release would either drift the schema or force an API
// change for a flag the catalog never needed semantically. An annotation has no
// schema, no CEL, and no regeneration, so adopting a new shaper flag is a data
// edit rather than an API change.
//
// `one-shot-man/shaper-args` is that annotation: a JSON array of strings
// appended, verbatim, to the ai-concurrency-shaper command line inside the
// access's mount (provider scope). It is the escape hatch for flags the
// product neither derives nor validates — `-opencode=true` on the opencode.ai
// mounts, for example. It is a string because every k8s annotation is a
// string, so the structure is encoded as JSON and MUST be validated by the
// consumer: the CRD cannot check it and this API deliberately does not
// pretend to.
//
// Every element is written in the `-flag=value` form, including booleans. A
// bare flag is not merely untidy: the shaper consumes the token after any
// value-taking flag written without `=`, so a bare element would swallow the
// NEXT mount's `--provider=` marker and merge the two mounts with no error
// reported. Requiring the `=` form makes that unrepresentable.
//
// The prefix is the API group, which is the Kubernetes convention for keys an
// API owns (the same reason metadata.name lives beside
// one-shot-man/registry-name rather than a vendor namespace): a consumer's own
// facts would use its own domain, as `example.com/owner` does in the module
// tests.
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

	// PreferredSurface names which served surface this access prefers when
	// mounted or resolved. Valid values: anthropic, responses, chat.
	// When absent, the product's standing surface preference applies.
	// +kubebuilder:validation:Enum=anthropic;responses;chat
	// +optional
	PreferredSurface string `json:"preferred_surface,omitempty"`

	// Surfaces explicitly lists the wire surfaces served by this access.
	// When omitted, surfaces are derived from the keys of Endpoints.
	// +kubebuilder:validation:items:Enum=anthropic;chat;responses
	// +optional
	Surfaces []string `json:"surfaces,omitempty"`

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

	// Shaper carries operational configuration for shaper-mode accesses.
	// +optional
	Shaper *ShaperConfig `json:"shaper,omitempty"`
}

// ShaperConfig carries the operational parameters for a shaper-mode access:
// the upstream URL, auth and transcode behaviour, and concurrency/retry/circuit-breaker
// tuning. All fields are optional; absent fields let the shaper binary's defaults apply.
type ShaperConfig struct {
	// Upstream is the real upstream URL the shaper forwards to. The
	// spec.endpoints URLs are client-facing (naming the gateway's own
	// address), so this field carries the actual provider endpoint.
	// Required for shaper-mode accesses.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Upstream string `json:"upstream,omitempty"`

	// AuthMode is the upstream auth mode: auto, none, bearer, x-api-key, api-key, header:NAME.
	// +optional
	AuthMode string `json:"auth_mode,omitempty"`

	// Transcode is the transcode preset override: auto, none, messages-chat.
	// +kubebuilder:validation:Enum=auto;none;messages-chat
	// +optional
	Transcode string `json:"transcode,omitempty"`

	// Concurrency is the max concurrent limited requests (-concurrency).
	// +kubebuilder:validation:Minimum=1
	// +optional
	Concurrency *int32 `json:"concurrency,omitempty"`

	// LimitAll enables limiting all requests, not just matching routes (-limit-all).
	// +optional
	LimitAll *bool `json:"limit_all,omitempty"`

	// QueueTimeout is the max time a request waits in the queue (-queue-timeout).
	// Duration string, e.g. "30m", "30s".
	// +optional
	QueueTimeout string `json:"queue_timeout,omitempty"`

	// Retry is the max retries for limited requests (-retry). Negative means unlimited.
	// +optional
	Retry *int32 `json:"retry,omitempty"`

	// RetryMinDelay is the minimum delay before retrying (-retry-min-delay). Duration string.
	// +optional
	RetryMinDelay string `json:"retry_min_delay,omitempty"`

	// RetrySkip429 skips retrying 429 responses (-retry-skip-429).
	// +optional
	RetrySkip429 *bool `json:"retry_skip_429,omitempty"`

	// ReleaseCooldown is the delay after slot release before re-admission (-release-cooldown). Duration string.
	// +optional
	ReleaseCooldown string `json:"release_cooldown,omitempty"`

	// CancelCooldown is the slot hold after client cancel (-cancel-cooldown). Duration string.
	// +optional
	CancelCooldown string `json:"cancel_cooldown,omitempty"`

	// CircuitBreaker enables the circuit breaker (-circuit-breaker).
	// +optional
	CircuitBreaker *bool `json:"circuit_breaker,omitempty"`

	// CBThreshold is failures within window to trip circuit breaker (-cb-threshold).
	// +kubebuilder:validation:Minimum=1
	// +optional
	CBThreshold *int32 `json:"cb_threshold,omitempty"`

	// CBWindow is the failure counting window (-cb-window). Duration string.
	// +optional
	CBWindow string `json:"cb_window,omitempty"`

	// CBPenalty is the base phantom concurrency hold time (-cb-penalty). Duration string.
	// +optional
	CBPenalty string `json:"cb_penalty,omitempty"`

	// CBMaxPenalty is the max phantom concurrency hold time (-cb-max-penalty). Duration string.
	// +optional
	CBMaxPenalty string `json:"cb_max_penalty,omitempty"`
}

// ModelAccessList is the list form of ModelAccess.
//
// +kubebuilder:object:root=true
type ModelAccessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ModelAccess `json:"items"`
}
