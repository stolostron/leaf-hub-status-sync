package transport

// Transport is the transport layer interface to be consumed by the leaf hub status sync.
type Transport interface {
	SendAsync(id string, msgType string, version string, payload []byte)
	GetVersion(id string, msgType string) string
}
