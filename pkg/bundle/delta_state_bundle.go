package bundle

// DeltaStateBundle provides functionalities specific to the delta-state sync bundles.
type DeltaStateBundle interface {
	HybridBundle
	// FlushOrderedObjects flushes a number of objects inside a bundle (from the beginning).
	FlushOrderedObjects(count int)
}
