package bundle

// HybridBundle is an abstraction for expanding Bundle interface to support hybrid-mode functionalities.
type HybridBundle interface {
	Bundle
	// GetObjects function to return the hybrid bundle's objects.
	GetObjects() interface{}
	// Enable function to sync bundle's recorded baseline generation and enable it for object updates.
	Enable()
	// Disable function to flush a bundle and prohibit it from taking object updates.
	Disable()
}
