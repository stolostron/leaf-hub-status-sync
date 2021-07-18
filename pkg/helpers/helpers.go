package helpers

import (
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	"strconv"
)

func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}

	return false
}

func GetBundleGenerationFromTransport(transport transport.Transport, msgId string, msgType string) uint64 {
	version := transport.GetVersion(msgId, msgType)
	if version == "" {
		return 0
	}
	generation, err := strconv.Atoi(version)
	if err != nil {
		return 0
	}
	return uint64(generation)
}
