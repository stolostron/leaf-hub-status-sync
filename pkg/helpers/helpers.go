package helpers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// RequeuePeriod is the time to wait until reconciliation retry in failure cases.
	RequeuePeriod = 5 * time.Second
)

// HasAnnotation returns a bool if the given annotation exists in annotations.
func HasAnnotation(obj metav1.Object, annotation string) bool {
	if obj == nil || obj.GetAnnotations() == nil {
		return false
	}

	_, found := obj.GetAnnotations()[annotation]

	return found
}

// HasLabel returns a bool if the given label exists in labels.
func HasLabel(obj metav1.Object, label string) bool {
	if obj == nil || obj.GetLabels() == nil {
		return false
	}

	_, found := obj.GetLabels()[label]

	return found
}

// SyncToTransport syncs the provided bundle to transport.
func SyncToTransport(transportObj transport.Transport, msgID string, msgType string, version *status.BundleVersion,
	payload bundle.Bundle) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to sync object from type %s with id %s - %w", msgType, msgID, err)
	}

	transportObj.SendAsync(&transport.Message{
		ID:      msgID,
		MsgType: msgType,
		Version: version,
		Payload: payloadBytes,
	})

	return nil
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
