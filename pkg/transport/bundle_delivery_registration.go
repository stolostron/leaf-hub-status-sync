package transport

import (
	"sync"
)

// DeliveryEvent is used to map conditions/actions to specific delivery events.
// When a condition check or a delivery event occurs, the appropriate conditions/actions are checked or invoked.
type DeliveryEvent uint8

const (
	// BeforeDeliveryAttempt is when a transportation operation is about to be called.
	BeforeDeliveryAttempt DeliveryEvent = iota
	// BeforeDeliveryRetry is when a transportation retry operation is about to be called.
	BeforeDeliveryRetry DeliveryEvent = iota
	// AfterDeliveryAttempt is when a transportation operation has just been called.
	AfterDeliveryAttempt DeliveryEvent = iota
	// DeliverySuccess is when a transportation operation is confirmed to have succeeded.
	DeliverySuccess DeliveryEvent = iota
)

// DeliveryArgType is used to map conditions/actions to specific argument types.
// When a condition check / delivery event is called with a certain DeliveryArgType, the appropriate conditions/actions
// are checked or called.
type DeliveryArgType uint8

const (
	// ArgTypeNone is for subscriptions that require no arguments (nil).
	ArgTypeNone DeliveryArgType = iota
	// ArgTypeBundleGeneration is for subscriptions that use the bundle's generation (uint64).
	ArgTypeBundleGeneration DeliveryArgType = iota
	// ArgTypeBundleObjectsCount is for subscriptions that use the bundle's objects count (int).
	ArgTypeBundleObjectsCount DeliveryArgType = iota
	// ArgTypeBundleType is for subscriptions that require the bundle's type (string).
	ArgTypeBundleType DeliveryArgType = iota
)

// NewBundleDeliveryRegistration creates a new instance of BundleDeliveryRegistration.
// if a channel expecting argument receives nil, relevant calls would not push to it.
func NewBundleDeliveryRegistration(lastSentBundleGen uint64, retryChan chan *Message,
	failChan chan *error) *BundleDeliveryRegistration {
	return &BundleDeliveryRegistration{
		failChan:                 failChan,
		retryChan:                retryChan,
		lastSentBundleGeneration: lastSentBundleGen,
		conditionSubscriptions:   make(map[DeliveryEvent]map[DeliveryArgType]func(interface{}) bool),
		actionSubscriptions:      make(map[DeliveryEvent]map[DeliveryArgType][]func(interface{})),
		subLock:                  sync.Mutex{},
		genLock:                  sync.Mutex{},
	}
}

// BundleDeliveryRegistration abstract the registration for different bundle delivery information and functionalities
// in one place, according to bundle key in transport layer.
type BundleDeliveryRegistration struct {
	failChan                 chan *error
	retryChan                chan *Message
	lastSentBundleGeneration uint64
	conditionSubscriptions   map[DeliveryEvent]map[DeliveryArgType]func(interface{}) bool
	actionSubscriptions      map[DeliveryEvent]map[DeliveryArgType][]func(interface{})
	subLock                  sync.Mutex
	genLock                  sync.Mutex
}

// AddAction adds a delegate action to be called when a relevant delivery-event occurs with the given arg type.
func (bdr *BundleDeliveryRegistration) AddAction(eventType DeliveryEvent, argumentType DeliveryArgType,
	action func(interface{})) {
	bdr.subLock.Lock()
	defer bdr.subLock.Unlock()

	actionsMap, found := bdr.actionSubscriptions[eventType]
	if !found {
		actionsMap = make(map[DeliveryArgType][]func(interface{}))
		bdr.actionSubscriptions[eventType] = actionsMap
	}

	actionsMap[argumentType] = append(actionsMap[argumentType], action)
}

// AddCondition adds a condition to be checked when a relevant check is called with the given arg type.
func (bdr *BundleDeliveryRegistration) AddCondition(eventType DeliveryEvent, argumentType DeliveryArgType,
	condition func(interface{}) bool) {
	bdr.subLock.Lock()
	defer bdr.subLock.Unlock()

	conditions, found := bdr.conditionSubscriptions[eventType]
	if !found {
		conditions = make(map[DeliveryArgType]func(interface{}) bool)
		bdr.conditionSubscriptions[eventType] = conditions
	}

	if existingCondition, found := conditions[argumentType]; found {
		// a condition already exists, append to it
		conditions[argumentType] = func(arg interface{}) bool {
			return condition(arg) && existingCondition(arg)
		}

		return
	}
	// first condition for this event-arg type pair
	conditions[argumentType] = func(arg interface{}) bool {
		return condition(arg)
	}
}

// InvokeEventActions asynchronously calls all actions subscribed to eventType that expect an argument of argType.
func (bdr *BundleDeliveryRegistration) InvokeEventActions(eventType DeliveryEvent, argType DeliveryArgType,
	arg interface{}) {
	bdr.subLock.Lock()
	defer bdr.subLock.Unlock()

	if actionsMap, eventFound := bdr.actionSubscriptions[eventType]; eventFound {
		for _, action := range actionsMap[argType] {
			go action(arg)
		}
	}
}

// CheckEventCondition checks conditions on eventType that expect an argument of argType.
func (bdr *BundleDeliveryRegistration) CheckEventCondition(eventType DeliveryEvent, argType DeliveryArgType,
	arg interface{}) bool {
	bdr.subLock.Lock()
	defer bdr.subLock.Unlock()

	if conditionMap, eventFound := bdr.conditionSubscriptions[eventType]; eventFound {
		if condition, found := conditionMap[argType]; found {
			return condition(arg)
		}
	}
	// if no condition exists for this eventType / argType, assume no one cares.
	return true
}

// UpdateSentGeneration updates the registered bundle's last sent generation.
func (bdr *BundleDeliveryRegistration) UpdateSentGeneration(generation uint64) {
	bdr.genLock.Lock()
	defer bdr.genLock.Unlock()

	bdr.lastSentBundleGeneration = generation
}

// GetLastSentGeneration returns the registered bundle's last sent generation.
func (bdr *BundleDeliveryRegistration) GetLastSentGeneration() uint64 {
	bdr.genLock.Lock()
	defer bdr.genLock.Unlock()

	return bdr.lastSentBundleGeneration
}

// PropagateFailure forwards error to the failure channel (blocking).
func (bdr *BundleDeliveryRegistration) PropagateFailure(err *error) {
	if bdr.failChan != nil {
		bdr.failChan <- err
	}
}

// PropagateRetry forwards message to the retry channel (blocking).
func (bdr *BundleDeliveryRegistration) PropagateRetry(msg *Message) {
	if bdr.retryChan != nil {
		bdr.retryChan <- msg
	}
}
