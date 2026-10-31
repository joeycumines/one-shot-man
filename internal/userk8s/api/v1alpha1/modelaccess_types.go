package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WireProtocol names the wire protocol a provider endpoint speaks natively.
// It is a fact about the provider/channel, not about a Model: every route that
// dials this access targets this protocol, and a route must not publish a
// client surface the protocol cannot reach.
//
// The values are the shaper's dialect vocabulary: chat is OpenAI Chat
// Completions, messages is the Anthropic Messages API, and responses is the
// OpenAI Responses API.
type WireProtocol string

const (
	// WireProtocolChat is OpenAI Chat Completions.
	WireProtocolChat WireProtocol = "chat"
	// WireProtocolMessages is the Anthropic Messages API.
	WireProtocolMessages WireProtocol = "messages"
	// WireProtocolResponses is the OpenAI Responses API.
	WireProtocolResponses WireProtocol = "responses"
)

// ModelAccess is one LOGICAL way of reaching a ModelProvider: the provider and
// the wire protocol. It carries no credentials, endpoints, transport, or
// models.
//
// Credentials are entity facts (LocalSecretBinding), and connection means are
// entity facts (ModelRoute). Keeping them off the access is what lets the same
// catalog be reached through a local shaper, a remote gateway, a sandbox
// tunnel, or a direct client without editing the logical catalog.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')",message="metadata.name must match ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
type ModelAccess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ModelAccessSpec `json:"spec,omitempty"`
}

// ModelAccessSpec is one logical provider/protocol channel.
type ModelAccessSpec struct {
	// Provider references a ModelProvider by name. Must resolve; the
	// cross-resource check is a loader/controller-side gap.
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`

	// Protocol is the wire protocol the provider endpoint speaks natively.
	// +kubebuilder:validation:Enum=chat;messages;responses
	Protocol WireProtocol `json:"protocol"`

	// Deprecated marks the access as no longer recommended for new routes.
	// +kubebuilder:default=false
	// +optional
	Deprecated bool `json:"deprecated,omitempty"`

	// Labels are Kubernetes-compatible labels. Credential bindings select
	// accesses/providers by label; they never attach to models.
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
