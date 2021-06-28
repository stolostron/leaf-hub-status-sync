package generic

import "github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"

func NewBundleCollectionEntry(transportBundleKey string, bundle bundle.Bundle) *BundleCollectionEntry {
	return &BundleCollectionEntry{
		transportBundleKey:       transportBundleKey,
		bundle:                   bundle,
		lastSentBundleGeneration: bundle.GetBundleGeneration(),
	}
}

type BundleCollectionEntry struct {
	transportBundleKey       string
	bundle                   bundle.Bundle
	lastSentBundleGeneration uint64
}
