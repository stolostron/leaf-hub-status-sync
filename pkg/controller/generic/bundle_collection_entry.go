package generic

import (
	"github.com/stolostron/hub-of-hubs-data-types/bundle/status"
	"github.com/stolostron/leaf-hub-status-sync/pkg/bundle"
)

// NewBundleCollectionEntry creates a new instance of BundleCollectionEntry.
func NewBundleCollectionEntry(transportBundleKey string, bundle bundle.Bundle,
	predicate func() bool) *BundleCollectionEntry {
	return &BundleCollectionEntry{
		transportBundleKey:    transportBundleKey,
		bundle:                bundle,
		predicate:             predicate,
		lastSentBundleVersion: *bundle.GetBundleVersion(),
	}
}

// BundleCollectionEntry holds information about a specific bundle.
type BundleCollectionEntry struct {
	transportBundleKey    string
	bundle                bundle.Bundle
	predicate             func() bool
	lastSentBundleVersion status.BundleVersion // not pointer so it does not point to the bundle's internal version
}
