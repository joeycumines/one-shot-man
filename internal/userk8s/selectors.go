package userk8s

import (
	"maps"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// accessLabelSet is the label set a LocalSecretBinding selector matches: the
// access's metadata labels merged with the mirroring spec labels.
func accessLabelSet(access *v1alpha1.ModelAccess) labels.Set {
	set := labels.Set{}
	maps.Copy(set, access.Spec.Labels)
	maps.Copy(set, access.Labels)
	return set
}

// selectorMatches applies a Kubernetes LabelSelector to a label set. A nil
// selector matches nothing, matching the CRD rule that an absent selector is
// rejected by validation rather than treated as match-all.
func selectorMatches(sel *metav1.LabelSelector, set labels.Set) (bool, error) {
	if sel == nil {
		return false, nil
	}
	selector, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return false, err
	}
	return selector.Matches(set), nil
}
