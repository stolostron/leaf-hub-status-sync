package helpers

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// RequeuePeriodSeconds is the time to wait until reconciliation retry in failure cases.
	RequeuePeriodSeconds = 5
	// base10 is the base used to cast string to int.
	base10 = 10
)

// GetGenerationFromTransport returns bundle generation from transport layer.
func GetGenerationFromTransport(transport transport.Transport, msgID string, msgType string) uint64 {
	version := transport.GetVersion(msgID, msgType)
	if version == "" {
		return 0
	}

	generation, err := strconv.Atoi(version)
	if err != nil {
		return 0
	}

	return uint64(generation)
}

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
func SyncToTransport(transport transport.Transport, msgID string, msgType string, generation uint64,
	payload bundle.Bundle) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to sync object from type %s with id %s- %w", msgType, msgID, err)
	}

	version := strconv.FormatUint(generation, base10)
	transport.SendAsync(msgID, msgType, version, payloadBytes)

	return nil
}
