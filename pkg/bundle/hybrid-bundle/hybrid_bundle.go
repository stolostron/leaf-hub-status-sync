package hybridbundle

import "github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"

// HybridBundle abstracts the functionality needed from a bundle to be synced in hybrid mode.
type HybridBundle interface {
	bundle.Bundle
	// GetBundleType returns a pointer to the bundle key (changes based on mode).
	GetBundleType() string
	// HandleBundleTransportationAttempt lets the bundle know that it was transported.
	HandleBundleTransportationAttempt()
	// HandleTransportationSuccess lets the bundle know that the latest transportation was successful.
	HandleTransportationSuccess()
	// HandleTransportationFailure lets the bundle know that the latest transportation failed.
	HandleTransportationFailure()
}
