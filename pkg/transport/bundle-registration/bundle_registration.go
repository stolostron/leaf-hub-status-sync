package bundleregistration

import "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"

// BundleRegistration abstracts managing transport-related info for bundles.
type BundleRegistration interface {
	// GetBundleTransportKey returns the bundle's registered transport key.
	GetBundleTransportKey() string
	// GetLastSentBundleVersion returns the bundle's last transported version.
	GetLastSentBundleVersion() *status.BundleVersion
	// SetLastSentBundleVersion sets the bundle's last transported version.
	SetLastSentBundleVersion(version *status.BundleVersion)
	// HandleFailure handles transportation failure.
	HandleFailure()
	// HandleSuccess handles transportation success.
	HandleSuccess()
}
