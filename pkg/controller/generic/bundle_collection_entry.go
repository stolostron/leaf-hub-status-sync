package generic

import (
	"fmt"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"strings"
)

// NewBundleCollectionEntry creates a new instance of BundleCollectionEntry.
func NewBundleCollectionEntry(transportBundleKey string, bundle bundle.Bundle,
	predicate func() bool) *BundleCollectionEntry {
	return &BundleCollectionEntry{
		transportBundleKey:       transportBundleKey,
		bundle:                   bundle,
		predicate:                predicate,
		lastSentBundleGeneration: bundle.GetBundleGeneration(),
	}
}

func (entry *BundleCollectionEntry) ChangeLeafHubName(leafHubNameIndex int) {
	var tokens = strings.Split(entry.transportBundleKey, ".")
	var newLeafHubName = fmt.Sprintf("%s_%d", tokens[0], leafHubNameIndex)

	entry.transportBundleKey = fmt.Sprintf("%s.%s", newLeafHubName, tokens[1])
	entry.bundle.ChangeLeafHubName(newLeafHubName)
}

// BundleCollectionEntry holds information about a specific bundle.
type BundleCollectionEntry struct {
	transportBundleKey       string
	bundle                   bundle.Bundle
	predicate                func() bool
	lastSentBundleGeneration uint64
}
