package helpers

import (
	"strconv"

	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

// ContainsString returns true if the string exists in the array and false otherwise
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}

	return false
}

// GetBundleGenerationFromTransport returns bundle generation from transport layer
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
