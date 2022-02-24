package helpers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/stolostron/leaf-hub-status-sync/pkg/bundle"
	"github.com/stolostron/leaf-hub-status-sync/pkg/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// RequeuePeriod is the time to wait until reconciliation retry in failure cases.
	RequeuePeriod = 5 * time.Second
	// RootPolicyLabel a label used to point to the root policy. if this label appears on a policy, it's not the root.
	RootPolicyLabel = "policy.open-cluster-management.io/root-policy"
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
func SyncToTransport(transportObj transport.Transport, msgID string, msgType string, version fmt.Stringer,
	payload bundle.Bundle) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to sync object from type %s with id %s - %w", msgType, msgID, err)
	}

	transportMessageKey := msgID

	if deltaStateBundle, ok := payload.(bundle.DeltaStateBundle); ok {
		transportMessageKey = fmt.Sprintf("%s@%d", msgID, deltaStateBundle.GetTransportationID())
	}

	transportObj.SendAsync(&transport.Message{
		Key:     transportMessageKey,
		ID:      msgID,
		MsgType: msgType,
		Version: version.String(),
		Payload: payloadBytes,
	})

	return nil
}
