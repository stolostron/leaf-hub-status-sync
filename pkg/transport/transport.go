package transport

// Transport is the transport layer interface to be consumed by the leaf hub status sync.
type Transport interface {
	SendAsync(message *Message)
	Register(msgType string, deliveryRegistration *BundleDeliveryRegistration)
	Start()
	Stop()
}
