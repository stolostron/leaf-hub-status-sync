package helpers

import (
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ContainsString returns true if the string exists in the array and false otherwise.
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}

	return false
}

// HasAnnotation returns a bool if the given annotation exists in annotations.
func HasAnnotation(obj metav1.Object, annotation string) bool {
	if obj == nil || obj.GetAnnotations() == nil {
		return false
	}

	_, found := obj.GetAnnotations()[annotation]

	return found
}

// AddConditionToDeliveryRegistrations adds given condition to list of delivery registrations.
func AddConditionToDeliveryRegistrations(deliveryRegistrations []*transport.BundleDeliveryRegistration,
	eventType transport.DeliveryEvent, argType transport.DeliveryArgType, condition func(interface{}) bool) {
	for _, deliveryRegistration := range deliveryRegistrations {
		deliveryRegistration.AddCondition(eventType, argType, condition)
	}
}

// Min returns min(a, b).
func Min(a, b int) int {
	if a > b {
		return b
	}

	return a
}
