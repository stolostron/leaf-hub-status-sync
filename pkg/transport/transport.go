package transport

import (
	bundleregistration "github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport/bundle-registration"
)

// Transport is the transport layer interface to be consumed by the leaf hub status sync.
type Transport interface {
	// SendAsync sends a message to the transport component asynchronously.
	SendAsync(message *Message)
	// Register links a registration to a bundle ID to propagate transportation success/fail events.
	Register(msgID string, registration bundleregistration.BundleRegistration)
	// Start starts the transport.
	Start()
	// Stop stops the transport.
	Stop()
}
