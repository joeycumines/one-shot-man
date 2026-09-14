package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Model is one model as served via one access. Models are own resources, not
// children of providers; the same underlying model served through two
// accesses is two resources linked by spec.equivalent_to. Cluster-scoped.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')",message="metadata.name must match ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ — emitted resource names are sanitized; the raw registry model id lives in the one-shot-man/registry-name annotation"
type Model struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ModelSpec `json:"spec,omitempty"`
}

// EquivalentModelRef references another registry model that serves the same
// underlying model under different constraints.
type EquivalentModelRef struct {
	// Provider is the registry provider name of the equivalent model.
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`

	// Name is the registry model identifier of the equivalent model.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ModelToolSettings holds sparse per-tool settings overriding the model's
// hard facts for one tool. A setting that exceeds a fact requires a non-empty
// waive reason; the compromise is then explicit and auditable.
type ModelToolSettings struct {
	// ContextWindow overrides the context budget for this tool. Exceeding
	// the model's context_window requires a waive reason.
	// +optional
	ContextWindow *int64 `json:"context_window,omitempty"`

	// MaxOutputTokens overrides the output budget for this tool. Exceeding
	// the model's max_output_tokens requires a waive reason.
	// +optional
	MaxOutputTokens *int64 `json:"max_output_tokens,omitempty"`

	// AutoCompactWindow overrides how much context the tool compacts at.
	// Exceeding the model's context_window requires a waive reason.
	// +optional
	AutoCompactWindow *int64 `json:"auto_compact_window,omitempty"`

	// EffortLevel pins the reasoning effort this tool requests. When the
	// model publishes its accepted levels, the value must be one of them.
	// +optional
	EffortLevel *string `json:"effort_level,omitempty"`

	// Flags carries tool-specific boolean or string knobs by name.
	// +optional
	Flags map[string]string `json:"flags,omitempty"`

	// Waive is the reason a budget override intentionally exceeds the
	// model's hard fact. Required for such overrides.
	// +optional
	Waive string `json:"waive,omitempty"`
}

// ModelSpec is the fact sheet of one model served through one access.
//
// +kubebuilder:validation:XValidation:rule="self.context_window > 0",message="spec.context_window must be positive — the provider-enforced context ceiling in tokens"
// +kubebuilder:validation:XValidation:rule="self.max_output_tokens > 0",message="spec.max_output_tokens must be positive — the provider-enforced maximum output budget in tokens"
// +kubebuilder:validation:XValidation:rule="!has(self.input_modalities) || self.input_modalities.all(m, m in ['text', 'image', 'audio'])",message="spec.input_modalities items must be one of text|image|audio"
// +kubebuilder:validation:XValidation:rule="!has(self.reasoning_efforts) || self.reasoning_efforts.all(e, e in ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra'])",message="spec.reasoning_efforts items must be one of none|minimal|low|medium|high|xhigh|max|ultra"
// +kubebuilder:validation:XValidation:rule="!has(self.toolSettings) || self.toolSettings.all(k, v, !has(v.context_window) || (v.context_window > 0 && (v.context_window <= self.context_window || (has(v.waive) && v.waive.size() > 0))))",message="tool settings context_window must be positive and must not exceed the model context_window without a waive reason"
// +kubebuilder:validation:XValidation:rule="!has(self.toolSettings) || self.toolSettings.all(k, v, !has(v.max_output_tokens) || (v.max_output_tokens > 0 && (v.max_output_tokens <= self.max_output_tokens || (has(v.waive) && v.waive.size() > 0))))",message="tool settings max_output_tokens must be positive and must not exceed the model max_output_tokens without a waive reason"
// +kubebuilder:validation:XValidation:rule="!has(self.toolSettings) || self.toolSettings.all(k, v, !has(v.auto_compact_window) || (v.auto_compact_window > 0 && (v.auto_compact_window <= self.context_window || (has(v.waive) && v.waive.size() > 0))))",message="tool settings auto_compact_window must be positive and must not exceed the model context_window without a waive reason"
// +kubebuilder:validation:XValidation:rule="!has(self.toolSettings) || self.toolSettings.all(k, v, !has(v.effort_level) || v.effort_level in ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra'])",message="tool settings effort_level must be one of none|minimal|low|medium|high|xhigh|max|ultra"
// +kubebuilder:validation:XValidation:rule="!has(self.toolSettings) || self.toolSettings.all(k, v, !has(v.effort_level) || !has(self.reasoning_efforts) || v.effort_level in self.reasoning_efforts)",message="tool settings effort_level must be one of the model's published reasoning_efforts"
type ModelSpec struct {
	// Provider references a ModelProvider by name. Must resolve; the
	// cross-resource check is a controller-side gap, not expressible in CEL.
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`

	// Access references a ModelAccess by name. Required when the provider has
	// multiple accesses; the cross-resource check is a controller-side gap.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Access string `json:"access,omitempty"`

	// ContextWindow is the provider-enforced context ceiling in tokens.
	// +kubebuilder:validation:Minimum=1
	ContextWindow int64 `json:"context_window"`

	// MaxOutputTokens is the provider-enforced maximum output budget.
	// +kubebuilder:validation:Minimum=1
	MaxOutputTokens int64 `json:"max_output_tokens"`

	// CanReason is true when the model supports reasoning/thinking.
	// +kubebuilder:default=false
	// +optional
	CanReason bool `json:"can_reason,omitempty"`

	// InputModalities lists the input modalities the model accepts.
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:items:Enum=text;image;audio
	// +kubebuilder:default={text}
	// +optional
	InputModalities []string `json:"input_modalities,omitempty"`

	// ReasoningEfforts is the exact set of reasoning effort levels the
	// provider accepts, present only when the provider publishes them.
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:items:Enum=none;minimal;low;medium;high;xhigh;max;ultra
	// +optional
	ReasoningEfforts []string `json:"reasoning_efforts,omitempty"`

	// Default marks the ordered preference for this provider: at most one
	// model per provider may set it; the cross-resource check is a
	// controller-side gap.
	// +kubebuilder:default=false
	// +optional
	Default bool `json:"default,omitempty"`

	// Deprecated marks a model no longer recommended; renderers skip
	// deprecated models unless explicitly pinned.
	// +kubebuilder:default=false
	// +optional
	Deprecated bool `json:"deprecated,omitempty"`

	// EquivalentTo links registry models that are the same underlying model
	// served under different constraints. The closure must be symmetric; the
	// cross-resource check is a controller-side gap.
	// +optional
	EquivalentTo []EquivalentModelRef `json:"equivalent_to,omitempty"`

	// Labels are Kubernetes-compatible labels, mirroring metadata.labels.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// ToolSettings holds sparse per-tool overrides keyed by tool name.
	// +optional
	ToolSettings map[string]ModelToolSettings `json:"toolSettings,omitempty"`
}

// ModelList is the list form of Model.
//
// +kubebuilder:object:root=true
type ModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Model `json:"items"`
}
