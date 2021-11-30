package generic

import (
	"github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

// NewBundleCollectionEntry creates a new instance of BundleCollectionEntry.
func NewBundleCollectionEntry(transportBundleKey string, bundle bundle.Bundle,
	deliveryRegistration *transport.BundleDeliveryRegistration) *BundleCollectionEntry {
	// add transportation conditions shared for all bundles:
	// -- BeforeDeliveryAttempt -- transport bundle only if its generation went up:
	deliveryRegistration.AddCondition(transport.BeforeDeliveryAttempt, transport.ArgTypeBundleVersion,
		func(version interface{}) bool {
			return (version.(*status.BundleVersion)).NewerThan(deliveryRegistration.GetLastSentBundleVersion())
		})
	// -- BeforeDeliveryRetry -- retry only latest (failed) version for bundle:
	deliveryRegistration.AddCondition(transport.BeforeDeliveryRetry, transport.ArgTypeBundleVersion,
		func(version interface{}) bool {
			return (version.(*status.BundleVersion)).Equals(deliveryRegistration.GetLastSentBundleVersion())
		})

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
