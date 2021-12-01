package hybridbundle

import "github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"

// CompleteStateBundle abstracts the logic needed from the complete-state bundle in a HybridBundle imp.
type CompleteStateBundle interface {
	bundle.Bundle
	// GetObjects returns the bundle's objects.
	GetObjects() interface{}
	// GetBundleType returns a pointer to the bundle key (changes based on mode).
	GetBundleType() string
}
