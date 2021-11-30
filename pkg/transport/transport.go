package transport

// Transport is the transport layer interface to be consumed by the leaf hub status sync.
type Transport interface {
	// SendAsync sends a message to the transport component asynchronously.
	SendAsync(message *Message)
	// Start starts the transport.
	Start()
	// Stop stops the transport.
	Stop()
}
