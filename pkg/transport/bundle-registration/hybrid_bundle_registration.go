package bundleregistration

import (
	"fmt"

	"github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
	hybridbundle "github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle/hybrid-bundle"
)

// NewHybridBundleRegistration returns a new instance of HybridBundleRegistration.
func NewHybridBundleRegistration(bundleTransportKeyPrefix string,
	hybridBundle hybridbundle.HybridBundle) BundleRegistration {
	return &HybridBundleRegistration{
		hybridBundle:             hybridBundle,
		bundleTransportKeyPrefix: bundleTransportKeyPrefix,
		lastSentBundleVersionMap: make(map[string]*status.BundleVersion),
	}
}

// HybridBundleRegistration implements BundleRegistration with hybrid sync based logic.
type HybridBundleRegistration struct {
	hybridBundle             hybridbundle.HybridBundle
	bundleTransportKeyPrefix string
	lastSentBundleVersionMap map[string]*status.BundleVersion
}

// GetBundleTransportKey returns the bundle's registered transport key.
func (reg *HybridBundleRegistration) GetBundleTransportKey() string {
	return fmt.Sprintf("%s%s", reg.bundleTransportKeyPrefix, reg.hybridBundle.GetBundleType())
}

// GetLastSentBundleVersion returns the bundle's last transported version.
func (reg *HybridBundleRegistration) GetLastSentBundleVersion() *status.BundleVersion {
	currentType := reg.hybridBundle.GetBundleType()

	if _, found := reg.lastSentBundleVersionMap[currentType]; !found {
		reg.lastSentBundleVersionMap[currentType] = status.NewBundleVersion(0, 0)
	}

	return reg.lastSentBundleVersionMap[currentType]
}

// SetLastSentBundleVersion sets the bundle's last transported version.
func (reg *HybridBundleRegistration) SetLastSentBundleVersion(version *status.BundleVersion) {
	reg.lastSentBundleVersionMap[reg.hybridBundle.GetBundleType()] = version

	reg.hybridBundle.HandleBundleTransportationAttempt()
}

// HandleFailure handles transportation failure.
func (reg *HybridBundleRegistration) HandleFailure() {
	reg.hybridBundle.HandleTransportationFailure()
}

// HandleSuccess handles transportation success.
func (reg *HybridBundleRegistration) HandleSuccess() {
	reg.hybridBundle.HandleTransportationSuccess()
}
