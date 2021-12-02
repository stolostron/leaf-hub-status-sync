package transport

// InvokeCallback invokes relevant callback in the given events subscription map.
func InvokeCallback(eventSubscriptionMap map[string]map[EventType]EventCallback, messageID string,
	eventType EventType) {
	callbacks, found := eventSubscriptionMap[messageID]
	if !found {
		return
	}

	if callback, found := callbacks[eventType]; found {
		callback()
	}
}
