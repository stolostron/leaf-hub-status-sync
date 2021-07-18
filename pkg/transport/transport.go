package transport

type Transport interface {
	SendAsync(id string, msgType string, version string, payload []byte)
	GetVersion(id string, msgType string) string
}
