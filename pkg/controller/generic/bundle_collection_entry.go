package generic

import (
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

// NewBundleCollectionEntry creates a new instance of BundleCollectionEntry.
func NewBundleCollectionEntry(transportBundleKey string, bundle bundle.Bundle,
	deliveryRegistration *transport.BundleDeliveryRegistration) *BundleCollectionEntry {
	return &BundleCollectionEntry{
		transportBundleKey:   transportBundleKey,
		bundle:               bundle,
		deliveryRegistration: deliveryRegistration,
	}
}

// BundleCollectionEntry holds information about a specific bundle.
type BundleCollectionEntry struct {
	transportBundleKey   string
	bundle               bundle.Bundle
	deliveryRegistration *transport.BundleDeliveryRegistration
}
