package hybridbundle

import "github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"

// DeltaStateBundle abstracts the logic needed from the delta-state bundle in a HybridBundle imp.
type DeltaStateBundle interface {
	bundle.Bundle
	// GetObjects returns the bundle's objects.
	GetObjects() interface{}
	// GetBundleType returns a pointer to the bundle key (changes based on mode).
	GetBundleType() string
	// SyncWithBase syncs the delta-state bundle with its complete-state base.
	SyncWithBase()
	// FlushObjects flushes the delta-state bundle's objects.
	FlushObjects()
}
