package transport

// HybridSyncTransport is an extension of Transport that supports hybrid-sync mode (complete-state + delta-state sync).
type HybridSyncTransport interface {
	Transport
	// RegisterDeltaMessagePrefix registers a bundle type (delta-bundle) with a key prefix that should change per delta-
	// bundle to control the message-compaction if supported and enabled and avoid losing delta bundles.
	//
	// IMPORTANT:
	//
	// 1) this implies that when using this functionality, it must be fine that the reader (consumer) can lose
	// delta-messages due to compaction if the prefix resets.
	//
	// 2) the given prefix must be safe to read by the transport, alter in transport callbacks only.
	RegisterDeltaMessagePrefix(msgID string, prefix *int)
}
