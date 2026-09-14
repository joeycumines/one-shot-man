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
)

// GroupVersion is the group and version identifier for the catalog API.
var GroupVersion = schema.GroupVersion{Group: Group, Version: Version}
