package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Tool describes one AI tool the launcher can drive: its display name, the
// wire surfaces it speaks, the credential channels it accepts, identifier
// grammars for provider/model slugs, and the name of the budget derivation
// rule its adapter implements. Cluster-scoped. Tool mechanics live in adapter
// code; this resource only models the facts the catalog needs to project a
// selection into a tool-specific launch plan.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')",message="metadata.name must match ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ — emitted resource names are sanitized; the raw registry name lives in the one-shot-man/registry-name annotation"
type Tool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ToolSpec `json:"spec,omitempty"`
}

// ToolIdentifierGrammar is a named regular expression describing how a tool
// renders one class of slug (provider or model identifiers).
type ToolIdentifierGrammar struct {
	// Pattern is the RE2 regular expression a slug must match.
	// +kubebuilder:validation:MinLength=1
	Pattern string `json:"pattern"`

	// Description explains the grammar in operator terms.
	// +optional
	Description string `json:"description,omitempty"`
}

// ToolIdentifiers holds the slug grammars per identifier class.
type ToolIdentifiers struct {
	// ProviderSlug is the grammar for provider identifiers.
	ProviderSlug ToolIdentifierGrammar `json:"provider_slug"`

	// ModelSlug is the grammar for model identifiers.
	ModelSlug ToolIdentifierGrammar `json:"model_slug"`
}

// ToolSpec is the catalog-relevant fact sheet of one tool.
type ToolSpec struct {
	// DisplayName is the human-readable tool name (e.g. "Claude Code").
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"display_name"`

	// Surfaces lists the wire surfaces this tool can speak.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:items:Enum=anthropic;chat;responses
	Surfaces []string `json:"surfaces"`

	// CredentialChannels lists how this tool can receive credentials:
	// gateway (anonymous localhost gateway endpoints), envRef (env-var
	// references the tool resolves), env (direct environment variables).
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:UniqueItems=true
	// +kubebuilder:validation:items:Enum=gateway;envRef;env
	CredentialChannels []string `json:"credential_channels"`

	// Identifiers holds the provider/model slug grammars.
	Identifiers ToolIdentifiers `json:"identifiers"`

	// BudgetProfile names the adapter-implemented budget derivation rule
	// (e.g. claude-margin, sdk-limits). The configured rule lives in adapter
	// code; the catalog only names it.
	// +kubebuilder:validation:MinLength=1
	BudgetProfile string `json:"budget_profile"`
}

// ToolList is the list form of Tool.
//
// +kubebuilder:object:root=true
type ToolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Tool `json:"items"`
}
