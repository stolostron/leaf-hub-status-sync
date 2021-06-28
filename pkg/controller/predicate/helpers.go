package predicate

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// hasAnnotation returns a bool if the given annotation exists in annotations
func hasAnnotation(obj metav1.Object, annotation string) bool {
	if obj == nil || obj.GetAnnotations() == nil {
		return false
	}
	_, found := obj.GetAnnotations()[annotation]
	return found
}

func hasFinalizer(obj metav1.Object, finalizer string) bool {
	if obj == nil || obj.GetFinalizers() == nil {
		return false
	}
	for _, finalizerInObj := range obj.GetFinalizers() {
		if finalizer == finalizerInObj {
			return true
		}
	}
	return false
}
