// Package v1alpha1 contains the one-shot-man model catalog API: the five
// custom resources (ModelProvider, ModelAccess, Model, LocalSecretBinding,
// Tool) that osm reads to resolve providers, models, and credentials.
//
// The Go types are the canonical definition; the committed CRD manifests
// under internal/userk8s/api/config/crd are generated from them by
// `gmake generate-crds` and verified by `gmake check-crds`.
//
// +kubebuilder:object:generate=true
// +groupName=one-shot-man
package v1alpha1
