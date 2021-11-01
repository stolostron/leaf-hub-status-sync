package bundle

// DeltaStateBundle provides functionalities specific to the delta-state sync bundles.
type DeltaStateBundle interface {
	HybridBundle
	// GetObjectsCount function to get bundle objects count.
	GetObjectsCount() int
	// FlushOrderedObjects flushes a number of objects inside a bundle (from the beginning).
	FlushOrderedObjects(count int)
}
