package generic

import (
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
)

// NewBundleCollectionEntry creates a new instnace of BundleCollectionEntry.
func NewBundleCollectionEntry(transportBundleKey string, bundle bundle.Bundle,
	predicate func() bool) *BundleCollectionEntry {
	return &BundleCollectionEntry{
		transportBundleKey:       transportBundleKey,
		bundle:                   bundle,
		predicate:                predicate,
		lastSentBundleGeneration: bundle.GetBundleGeneration(),
	}
}

// BundleCollectionEntry holds information about a specific bundle.
type BundleCollectionEntry struct {
	transportBundleKey       string
	bundle                   bundle.Bundle
	predicate                func() bool
	lastSentBundleGeneration uint64
}
