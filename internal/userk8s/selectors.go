package userk8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

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
