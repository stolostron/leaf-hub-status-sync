package bundle

import (
	"sync"

	statusbundle "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
)

// NewControlInfoBundle creates a new instance of ControlInfoBundle.
func NewControlInfoBundle(leafHubName string, incarnation uint64) *ControlInfoBundle {
	return &ControlInfoBundle{
		LeafHubName:   leafHubName,
		BundleVersion: statusbundle.NewBundleVersion(incarnation, 0),
		lock:          sync.Mutex{},
	}
}

// ControlInfoBundle holds control info passed from LH to HoH.
type ControlInfoBundle struct {
	LeafHubName   string                      `json:"leafHubName"`
	BundleVersion *statusbundle.BundleVersion `json:"bundleVersion"`
	lock          sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *ControlInfoBundle) UpdateObject(Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.BundleVersion.Generation++
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *ControlInfoBundle) DeleteObject(Object) {}

// GetBundleVersion function to get bundle version.
func (bundle *ControlInfoBundle) GetBundleVersion() *statusbundle.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.BundleVersion
}
