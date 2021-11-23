package bundle

import "sync"

// NewControlInfoBundle creates a new instance of ControlInfoBundle.
func NewControlInfoBundle(leafHubName string, generation uint64) *ControlInfoBundle {
	return &ControlInfoBundle{
		LeafHubName: leafHubName,
		Generation:  generation,
		lock:        sync.Mutex{},
	}
}

// ControlInfoBundle holds control info passed from LH to HoH.
type ControlInfoBundle struct {
	LeafHubName string `json:"leafHubName"`
	Generation  uint64 `json:"generation"`
	lock        sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *ControlInfoBundle) UpdateObject(Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.Generation++
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *ControlInfoBundle) DeleteObject(Object) {}

// GetBundleGeneration function to get bundle generation.
func (bundle *ControlInfoBundle) GetBundleGeneration() uint64 {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.Generation
}
