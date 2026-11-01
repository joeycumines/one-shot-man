package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModelServing is one provider's concrete offer of a Model: the wire id that
// provider calls it, the provider-specific capability narrowings, and the
// offering's preference/deprecation. It is credential-agnostic and
// access-agnostic: a ModelAccess is a logical channel and a ModelRoute binds
// an entity's credential/transport to this offering.
//
// metadata.name is the DEFAULT served id (the intermediary model id a tool may
// send). A route may expose the serving under a different id when the same
// offering is reachable through more than one access item.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')",message="metadata.name must match ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ — the served id is an RFC-1123 name; the catalog registry key lives in the one-shot-man/registry-name annotation"
type ModelServing struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ModelServingSpec `json:"spec,omitempty"`
}

// ModelServingOverrides are provider-specific capability narrowings. Every
// absent field inherits the Model. An override may only narrow the canonical
// definition: a provider that genuinely exceeds it means the Model is wrong and
// must be updated, not that the serving should widen.
type ModelServingOverrides struct {
	// ContextWindow is this provider's context ceiling; must not exceed the
	// Model's context_window.
	// +kubebuilder:validation:Minimum=1
	// +optional
	ContextWindow *int64 `json:"context_window,omitempty"`

	// MaxOutputTokens is this provider's output ceiling; must not exceed the
	// Model's max_output_tokens.
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxOutputTokens *int64 `json:"max_output_tokens,omitempty"`

	// InputModalities is this provider's accepted input modalities; must be a
	// subset of the Model's.
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:items:Enum=text;image;audio
	// +optional
	InputModalities []string `json:"input_modalities,omitempty"`

	// OutputModalities is this provider's produced output modalities; must be a
	// subset of the Model's.
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:items:Enum=text;image;audio
	// +optional
	OutputModalities []string `json:"output_modalities,omitempty"`

	// Reasoning replaces the Model's reasoning fact sheet for this provider.
	// It may only narrow it (no new modes, efforts, or budget headroom).
	// +optional
	Reasoning *ModelReasoning `json:"reasoning,omitempty"`

	// ToolCall may turn tool calling off; it cannot turn on a capability the
	// Model does not have.
	// +optional
	ToolCall *bool `json:"tool_call,omitempty"`

	// StructuredOutput may turn structured output off; it cannot turn on a
	// capability the Model does not have.
	// +optional
	StructuredOutput *bool `json:"structured_output,omitempty"`
}

// ModelServingSpec is one provider's offer of a Model.
//
// +kubebuilder:validation:XValidation:rule="!has(self.overrides) || !has(self.overrides.context_window) || self.overrides.context_window > 0",message="overrides.context_window must be positive"
// +kubebuilder:validation:XValidation:rule="!has(self.overrides) || !has(self.overrides.max_output_tokens) || self.overrides.max_output_tokens > 0",message="overrides.max_output_tokens must be positive"
type ModelServingSpec struct {
	// Provider references the ModelProvider that offers this model. Must
	// resolve; the cross-resource check is a loader/controller-side gap.
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`

	// Model references the provider-independent Model definition.
	// +kubebuilder:validation:MinLength=1
	Model string `json:"model"`

	// ProviderModelID is the id the PROVIDER calls this model, sent upstream
	// verbatim. It is never exposed as a model id on a served route.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	ProviderModelID string `json:"provider_model_id"`

	// Priority orders offerings of the same Model when a selection expands
	// them; lower is preferred. Zero is the unset value and is rejected when
	// declared explicitly: the default replaces it with 100.
	// +kubebuilder:default=100
	// +kubebuilder:validation:Minimum=1
	// +optional
	Priority int32 `json:"priority,omitempty"`

	// Default marks at most one offering per provider as its preferred one.
	// +kubebuilder:default=false
	// +optional
	Default bool `json:"default,omitempty"`

	// Deprecated marks this provider offer as no longer recommended; the same
	// Model may still be offered by another provider.
	// +kubebuilder:default=false
	// +optional
	Deprecated bool `json:"deprecated,omitempty"`

	// Overrides narrow the Model's capabilities for this provider.
	// +optional
	Overrides *ModelServingOverrides `json:"overrides,omitempty"`

	// Labels are Kubernetes-compatible labels, mirroring metadata.labels.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// EffectivePriority returns the priority with the CRD default applied.
func (spec ModelServingSpec) EffectivePriority() int32 {
	if spec.Priority == 0 {
		return 100
	}
	return spec.Priority
}

// ModelServingList is the list form of ModelServing.
//
// +kubebuilder:object:root=true
type ModelServingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ModelServing `json:"items"`
}
