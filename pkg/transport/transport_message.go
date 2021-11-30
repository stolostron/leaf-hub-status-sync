package transport

import "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"

// Message abstracts a message object to be used by different transport components.
type Message struct {
	ID      string                `json:"id"`
	MsgType string                `json:"msgType"`
	Version *status.BundleVersion `json:"version"`
	Payload []byte                `json:"payload"`
}
