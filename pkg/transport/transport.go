package transport

// Transport is the transport layer interface to be consumed by the leaf hub status sync.
type Transport interface {
	// SendAsync sends a message to the transport component asynchronously.
	SendAsync(transportMessage *Message)
	// Register registers a BundleDeliveryRegistration for a bundle type.
	Register(bundleType string, deliveryRegistration *BundleDeliveryRegistration)
	// Start starts the transport.
	Start()
	// Stop stops the transport.
	Stop()
}
