package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModelProvider is the entity that serves models: a vendor API, an
// aggregator, or a gateway. Facts only; consumer decisions live on Tool and
// Model.toolSettings. Cluster-scoped: the registry is global by nature.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:validation:XValidation:rule="self.metadata.name.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')",message="metadata.name must match ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ — emitted resource names are sanitized; the raw registry name lives in the one-shot-man/registry-name annotation"
type ModelProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ModelProviderSpec `json:"spec,omitempty"`
}

// ModelProviderSpec is the fact sheet of one model provider.
type ModelProviderSpec struct {
	// DisplayName is the human-readable provider name (e.g. "Electron Hub").
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"display_name"`

	// Country is the informative ISO country code of the provider's
	// jurisdiction (e.g. "cn"). Informative only — never used for routing.
	// +kubebuilder:validation:MinLength=2
	// +kubebuilder:validation:MaxLength=3
	// +optional
	Country string `json:"country,omitempty"`

	// Deprecated marks a provider no longer recommended for new tool
	// declarations. Renderers skip deprecated providers unless pinned.
	// +kubebuilder:default=false
	// +optional
	Deprecated bool `json:"deprecated,omitempty"`

	// Labels are Kubernetes-compatible labels, mirroring metadata.labels.
	// Consumed by the engine's LabelSelector machinery.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// ModelProviderList is the list form of ModelProvider.
//
// +kubebuilder:object:root=true
type ModelProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ModelProvider `json:"items"`
}
