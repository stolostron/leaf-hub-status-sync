package helpers

import (
	"strconv"

	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RequeuePeriodSeconds is the time to wait until reconciliation retry in failure cases.
const RequeuePeriodSeconds = 5

// ContainsString returns true if the string exists in the array and false otherwise.
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}

	return false
}

// GetBundleGenerationFromTransport returns bundle generation from transport layer.
func GetBundleGenerationFromTransport(transport transport.Transport, msgID string, msgType string) uint64 {
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
