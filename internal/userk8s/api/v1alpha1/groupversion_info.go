package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// Group is the API group served by this package.
	Group = "one-shot-man"
	// Version is the API version served by this package.
	Version = "v1alpha1"
	// RegistryNameAnnotation preserves the raw registry identifier on emitted
	// resources whose metadata.name was sanitized to the strict grammar name
	// enforced by every kind's root validation rule.
	RegistryNameAnnotation = Group + "/registry-name"
	// ShaperArgsAnnotation carries, on a ModelRoute whose transport kind is
	// shaper, a JSON array of strings appended verbatim to the
	// ai-concurrency-shaper command line for that route. See
	// ShaperTransportConfig for why this is an annotation and not a spec field.
	//
	// The value is unvalidated by construction: an annotation is a string, so
	// the array is JSON-encoded and only the CONSUMER can check it. Declare it
	// under the API group, which is the Kubernetes convention for a key this
	// API owns.
	ShaperArgsAnnotation = Group + "/shaper-args"
)

// GroupVersion is the group and version identifier for the catalog API.
var GroupVersion = schema.GroupVersion{Group: Group, Version: Version}
