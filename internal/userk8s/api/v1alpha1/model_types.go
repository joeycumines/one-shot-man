package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Model is a provider-independent definition of a model's configuration and
// capabilities. It carries no provider, access, credential, or wire id: those
// are facts of a ModelServing (a provider's offer) and a ModelRoute (an
// entity's exposure). A canonical Model is the equivalence class; offerings
// that narrow its capabilities reference it instead of duplicating it.
// Cluster-scoped.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')",message="metadata.name must match ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ — emitted resource names are sanitized; the raw registry model id lives in the one-shot-man/registry-name annotation"
type Model struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ModelSpec `json:"spec,omitempty"`
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

// ModelReasoning is the reasoning capability fact sheet of one model. It
// replaces the previous flat can_reason/reasoning_efforts pair, which could
// express only one mode and silently lost the others:
//
//   - Supported records that the model reasons at all, including always-on
//     reasoning with no client-selectable mode.
//   - Toggle records an explicit on/off mode. The catalog exposes it as the
//     "none" (off) and "thinking" (on) variants.
//   - Efforts is the exact, ordered set of effort tiers the provider accepts.
//   - Budget is the Anthropic-style token-budget range, when the provider
//     accepts one.
//
// A client derives its variants from these facts; the catalog never invents a
// mode the model does not have.
//
// +kubebuilder:validation:XValidation:rule="self.supported || (!self.toggle && !has(self.efforts) && !has(self.budget))",message="a reasoning mode requires supported: true"
// +kubebuilder:validation:XValidation:rule="!has(self.efforts) || self.efforts.all(e, e in ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra'])",message="reasoning.efforts items must be one of none|minimal|low|medium|high|xhigh|max|ultra"
type ModelReasoning struct {
	// Supported is true when the model supports reasoning/thinking at all.
	// +kubebuilder:default=false
	// +optional
	Supported bool `json:"supported,omitempty"`

	// Toggle is true when the provider accepts an explicit on/off choice.
	// +kubebuilder:default=false
	// +optional
	Toggle bool `json:"toggle,omitempty"`

	// Efforts is the exact set of reasoning effort levels the provider
	// accepts, in the provider's declared order.
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:items:Enum=none;minimal;low;medium;high;xhigh;max;ultra
	// +optional
	Efforts []string `json:"efforts,omitempty"`

	// Budget is the accepted token-budget range when the provider exposes
	// Anthropic-style thinking budgets.
	// +optional
	Budget *ModelReasoningBudget `json:"budget,omitempty"`
}

// ModelReasoningBudget is the accepted thinking-token budget range. Both
// bounds are optional because the provider may publish only a ceiling.
//
// +kubebuilder:validation:XValidation:rule="!has(self.min) || self.min >= 0",message="reasoning.budget.min must be non-negative"
// +kubebuilder:validation:XValidation:rule="!has(self.max) || self.max > 0",message="reasoning.budget.max must be positive"
// +kubebuilder:validation:XValidation:rule="!has(self.min) || !has(self.max) || self.min <= self.max",message="reasoning.budget.min must not exceed reasoning.budget.max"
type ModelReasoningBudget struct {
	// Min is the smallest budget the provider accepts, in tokens.
	// +kubebuilder:validation:Minimum=0
	// +optional
	Min *int64 `json:"min,omitempty"`

	// Max is the largest budget the provider accepts, in tokens.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Max *int64 `json:"max,omitempty"`
}

// ModelSpec is the fact sheet of one model a provider offers.
//
// +kubebuilder:validation:XValidation:rule="self.context_window > 0",message="spec.context_window must be positive — the provider-enforced context ceiling in tokens"
// +kubebuilder:validation:XValidation:rule="self.max_output_tokens > 0",message="spec.max_output_tokens must be positive — the provider-enforced maximum output budget in tokens"
// +kubebuilder:validation:XValidation:rule="!has(self.input_modalities) || self.input_modalities.all(m, m in ['text', 'image', 'audio'])",message="spec.input_modalities items must be one of text|image|audio"
// +kubebuilder:validation:XValidation:rule="!has(self.reasoning) || !has(self.reasoning.efforts) || self.reasoning.efforts.all(e, e in ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra'])",message="spec.reasoning.efforts items must be one of none|minimal|low|medium|high|xhigh|max|ultra"
// +kubebuilder:validation:XValidation:rule="!has(self.output_modalities) || self.output_modalities.all(m, m in ['text', 'image', 'audio'])",message="spec.output_modalities items must be one of text|image|audio"
// +kubebuilder:validation:XValidation:rule="!has(self.toolSettings) || self.toolSettings.all(k, v, !has(v.context_window) || (v.context_window > 0 && (v.context_window <= self.context_window || (has(v.waive) && v.waive.size() > 0))))",message="tool settings context_window must be positive and must not exceed the model context_window without a waive reason"
// +kubebuilder:validation:XValidation:rule="!has(self.toolSettings) || self.toolSettings.all(k, v, !has(v.max_output_tokens) || (v.max_output_tokens > 0 && (v.max_output_tokens <= self.max_output_tokens || (has(v.waive) && v.waive.size() > 0))))",message="tool settings max_output_tokens must be positive and must not exceed the model max_output_tokens without a waive reason"
// +kubebuilder:validation:XValidation:rule="!has(self.toolSettings) || self.toolSettings.all(k, v, !has(v.auto_compact_window) || (v.auto_compact_window > 0 && (v.auto_compact_window <= self.context_window || (has(v.waive) && v.waive.size() > 0))))",message="tool settings auto_compact_window must be positive and must not exceed the model context_window without a waive reason"
// +kubebuilder:validation:XValidation:rule="!has(self.toolSettings) || self.toolSettings.all(k, v, !has(v.effort_level) || v.effort_level in ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra'])",message="tool settings effort_level must be one of none|minimal|low|medium|high|xhigh|max|ultra"
// +kubebuilder:validation:XValidation:rule="!has(self.toolSettings) || self.toolSettings.all(k, v, !has(v.effort_level) || !has(self.reasoning) || !has(self.reasoning.efforts) || v.effort_level in self.reasoning.efforts)",message="tool settings effort_level must be one of the model's published reasoning.efforts"
type ModelSpec struct {
	// CanonicalID is the provider-independent identity of the model, e.g.
	// "zhipuai/glm-5.3". It is the grouping key across providers and the value
	// a canonical-naming route can expose. It is NOT the wire id.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	CanonicalID string `json:"canonical_id"`

	// Family is the model family/line (e.g. "glm"), when the catalog publishes
	// one.
	// +optional
	Family string `json:"family,omitempty"`

	// DisplayName is the human-readable model name (e.g. "GLM-5.3").
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"display_name"`

	// Aliases are additional selection aliases for this Model (raw ids,
	// old spellings, provider-neutral names). Aliases are a naming aid, not a
	// second identity.
	// +optional
	Aliases []string `json:"aliases,omitempty"`

	// ContextWindow is the canonical context budget in tokens. A ModelServing
	// may narrow it, never widen it.
	// +kubebuilder:validation:Minimum=1
	ContextWindow int64 `json:"context_window"`

	// MaxOutputTokens is the canonical maximum output budget in tokens. A
	// ModelServing may narrow it, never widen it.
	// +kubebuilder:validation:Minimum=1
	MaxOutputTokens int64 `json:"max_output_tokens"`

	// Reasoning is the model's reasoning capability: whether it reasons, the
	// selectable modes, exact effort tiers, and token-budget range.
	// +optional
	Reasoning *ModelReasoning `json:"reasoning,omitempty"`

	// InputModalities lists the input modalities the model accepts.
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:items:Enum=text;image;audio
	// +kubebuilder:default={text}
	// +optional
	InputModalities []string `json:"input_modalities,omitempty"`

	// OutputModalities lists the output modalities the model produces.
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:items:Enum=text;image;audio
	// +kubebuilder:default={text}
	// +optional
	OutputModalities []string `json:"output_modalities,omitempty"`

	// ToolCall is true when the model accepts tool/function calls.
	// +kubebuilder:default=true
	// +optional
	ToolCall bool `json:"tool_call,omitempty"`

	// StructuredOutput is true when the model accepts structured output
	// constraints.
	// +kubebuilder:default=false
	// +optional
	StructuredOutput bool `json:"structured_output,omitempty"`

	// Deprecated marks the model itself as no longer recommended. A deprecated
	// Model hides every serving unless one is explicitly pinned.
	// +kubebuilder:default=false
	// +optional
	Deprecated bool `json:"deprecated,omitempty"`

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
