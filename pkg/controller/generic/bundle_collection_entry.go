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

func (entry *BundleCollectionEntry) Clone(leafHubName string) *BundleCollectionEntry {
	var tokens = strings.Split(entry.transportBundleKey, ".")
	var clonedTransportBundleKey = fmt.Sprintf("%s.%s", leafHubName, tokens[1])
	var clonedBundle = entry.bundle.Clone(leafHubName)

	return NewBundleCollectionEntry(clonedTransportBundleKey, clonedBundle, entry.predicate)
}

// BundleCollectionEntry holds information about a specific bundle.
type BundleCollectionEntry struct {
	transportBundleKey       string
	bundle                   bundle.Bundle
	predicate                func() bool
	lastSentBundleGeneration uint64
}
