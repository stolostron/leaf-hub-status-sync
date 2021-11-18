package helpers

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// RequeuePeriodSeconds is the time to wait until reconciliation retry in failure cases.
	RequeuePeriodSeconds = 5
	// Base10 is the base used to cast string to int.
	Base10 = 10
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

// SyncToTransport syncs the provided bundle to transport.
// The bundle is provided as marker interface to resolve "cycle dependency" build error.
func SyncToTransport(transport transport.Transport, msgID string, msgType string, generation string,
	payload interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to sync object from type %s with id %s- %s", msgType, msgID, err)
	}

	transport.SendAsync(msgID, msgType, generation, payloadBytes)

	return nil
}
