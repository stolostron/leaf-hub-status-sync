package generic

import (
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	bundleregistration "github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport/bundle-registration"
)

// NewBundleCollectionEntry creates a new instance of BundleCollectionEntry.
func NewBundleCollectionEntry(bundle bundle.Bundle, predicate func() bool,
	transportRegistration bundleregistration.BundleRegistration) *BundleCollectionEntry {
	return &BundleCollectionEntry{
		bundle:                bundle,
		predicate:             predicate,
		transportRegistration: transportRegistration,
	}
}

// BundleCollectionEntry holds information about a specific bundle.
type BundleCollectionEntry struct {
	bundle                bundle.Bundle
	predicate             func() bool
	transportRegistration bundleregistration.BundleRegistration
}
