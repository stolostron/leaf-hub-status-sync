package bundleregistration

import "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"

// NewFullStateBundleRegistration returns a new instance of FullStateBundleRegistration.
func NewFullStateBundleRegistration(bundleTransportKey string) BundleRegistration {
	return &FullStateBundleRegistration{
		bundleTransportKey:    bundleTransportKey,
		lastSentBundleVersion: status.NewBundleVersion(0, 0),
	}
}

// FullStateBundleRegistration implements BundleRegistration with full-state sync based logic.
type FullStateBundleRegistration struct {
	bundleTransportKey    string
	lastSentBundleVersion *status.BundleVersion
}

// GetBundleTransportKey returns the bundle's registered transport key.
func (reg *FullStateBundleRegistration) GetBundleTransportKey() string {
	return reg.bundleTransportKey
}

// GetLastSentBundleVersion returns the bundle's last transported version.
func (reg *FullStateBundleRegistration) GetLastSentBundleVersion() *status.BundleVersion {
	return reg.lastSentBundleVersion
}

// SetLastSentBundleVersion sets the bundle's last transported version.
func (reg *FullStateBundleRegistration) SetLastSentBundleVersion(version *status.BundleVersion) {
	reg.lastSentBundleVersion = version
}

// HandleFailure handles transportation failure.
func (reg *FullStateBundleRegistration) HandleFailure() {}

// HandleSuccess handles transportation success.
func (reg *FullStateBundleRegistration) HandleSuccess() {}
