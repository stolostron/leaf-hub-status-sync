package transport

// EventType is the type of transportation-events that may occur.
type EventType string

// EventCallback is the type for subscription callbacks.
type EventCallback func()

const (
	// DeliveryAttempt event occurs when an attempted transport-delivery operation is attempted (sent to servers).
	DeliveryAttempt EventType = "attempt"
	// DeliverySuccess event occurs when an attempted transport-delivery operation is successful (ack from servers).
	DeliverySuccess EventType = "success"
	// DeliveryFailure event occurs when an attempted transport-delivery operation fails.
	DeliveryFailure EventType = "failure"
)

// Transport is the transport layer interface to be consumed by the leaf hub status sync.
type Transport interface {
	// SendAsync sends a message to the transport component asynchronously.
	SendAsync(message *Message)
	// Subscribe adds a callback to be delegated when a given event occurs for a message with the given ID.
	Subscribe(messageID string, callbacks map[EventType]EventCallback)
	// Start starts the transport.
	Start()
	// Stop stops the transport.
	Stop()
}
