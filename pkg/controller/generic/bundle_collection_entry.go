package generic

import (
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
)

// NewBundleCollectionEntry creates a new instnace of BundleCollectionEntry.
func NewBundleCollectionEntry(transportBundleKey string, bundle bundle.Bundle,
	predicate func() bool, deliveryConsumerFunc func(int)) *BundleCollectionEntry {
	return &BundleCollectionEntry{
		transportBundleKey:       transportBundleKey,
		bundle:                   bundle,
		predicate:                predicate,
		deliveryConsumerFunc:     deliveryConsumerFunc,
		lastSentBundleGeneration: bundle.GetBundleGeneration(),
	}
}

// BundleCollectionEntry holds information about a specific bundle.
type BundleCollectionEntry struct {
	transportBundleKey   string
	bundle               bundle.Bundle
	predicate            func() bool
	deliveryConsumerFunc func(int)
	// DO NOT WRITE TO THIS FIELD WITHIN THIS PACKAGE
	lastSentBundleGeneration uint64
}

// updateLastSentBundleGeneration updates the last-sent-bundle-generation and invokes delivery consumption functions.
func (bce *BundleCollectionEntry) updateLastSentBundleGeneration(sentBundleGeneration uint64, sentObjectsCount int) {
	bce.lastSentBundleGeneration = sentBundleGeneration
	bce.deliveryConsumerFunc(sentObjectsCount)
}
