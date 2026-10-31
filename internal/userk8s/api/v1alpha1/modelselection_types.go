package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModelSelectionStrategy names how a selection's candidates are resolved.
type ModelSelectionStrategy string

const (
	// SelectionOrdered resolves candidates in their declared order.
	SelectionOrdered ModelSelectionStrategy = "ordered"
	// SelectionPriority expands model candidates and orders the whole set by
	// serving priority, then candidate order.
	SelectionPriority ModelSelectionStrategy = "priority"
	// SelectionDynamic reserves request-aware, synthetic-id selection for a
	// future gateway suite router. The current gateway/launcher refuses it.
	SelectionDynamic ModelSelectionStrategy = "dynamic"
)

// ModelSelectionOfferRef names one route offer: the route (access item) and
// the served id it exposes.
type ModelSelectionOfferRef struct {
	// Route is the ModelRoute metadata name.
	// +kubebuilder:validation:MinLength=1
	Route string `json:"route"`

	// ID is the served id the route exposes.
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`
}

// ModelSelectionCandidate is one fallback candidate. Exactly one of the
// reference fields is set.
//
// +kubebuilder:validation:XValidation:rule="(has(self.serving) ? 1 : 0) + (has(self.model) ? 1 : 0) + (has(self.selection) ? 1 : 0) + (has(self.offer) ? 1 : 0) == 1",message="a candidate must set exactly one of serving, model, selection, or offer"
type ModelSelectionCandidate struct {
	// Serving names a concrete ModelServing. Every live route exposing it is
	// a candidate.
	// +optional
	Serving string `json:"serving,omitempty"`

	// Model names a Model. Its offerings (ModelServing resources) are expanded
	// in priority order, then their live routes.
	// +optional
	Model string `json:"model,omitempty"`

	// Selection names another ModelSelection (cycle-checked).
	// +optional
	Selection string `json:"selection,omitempty"`

	// Offer names one route offer, pinning both the access item and the
	// served id.
	// +optional
	Offer *ModelSelectionOfferRef `json:"offer,omitempty"`
}

// ModelSelection is an optional, entity-scoped fallback chain over any
// selectables. It is model-agnostic: a candidate may be a concrete serving, a
// canonical Model (expanded to its offerings), another selection, or a
// specific route offer — so a fallback is not limited to one model.
//
// The launcher resolves a selection against live routes and the tool's
// surfaces before composing, and configures the tool with the chosen concrete
// served id. A route may expose the selection id itself for a future dynamic
// gateway; the current suite has no cross-route runtime fallback.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:validation:XValidation:rule="self.metadata.name.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')",message="metadata.name must match ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
type ModelSelection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ModelSelectionSpec `json:"spec,omitempty"`
}

// ModelSelectionSpec is an ordered/priority fallback chain.
//
// +kubebuilder:validation:XValidation:rule="self.candidates.size() > 0",message="spec.candidates must name at least one candidate"
type ModelSelectionSpec struct {
	// DisplayName is the human-readable name of the selection.
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"display_name"`

	// Strategy is ordered or priority today; dynamic is reserved for a future
	// request-aware gateway router.
	// +kubebuilder:validation:Enum=ordered;priority;dynamic
	// +kubebuilder:default=ordered
	// +optional
	Strategy ModelSelectionStrategy `json:"strategy,omitempty"`

	// Candidates are the fallback chain, in preference order for ordered
	// selection.
	// +kubebuilder:validation:MinItems=1
	Candidates []ModelSelectionCandidate `json:"candidates"`
}

// ModelSelectionList is the list form of ModelSelection.
//
// +kubebuilder:object:root=true
type ModelSelectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ModelSelection `json:"items"`
}
