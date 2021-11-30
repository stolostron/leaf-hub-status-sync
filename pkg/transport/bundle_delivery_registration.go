package transport

import (
	"sync"

	"github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
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
	// ArgTypeBundleVersion is for subscriptions that use the bundle's version (*status.BundleVersion).
	ArgTypeBundleVersion DeliveryArgType = iota
)

// NewBundleDeliveryRegistration creates a new instance of BundleDeliveryRegistration.
// if a channel expecting argument receives nil, relevant calls would not push to it.
func NewBundleDeliveryRegistration(retryChan chan *Message, failChan chan *error) *BundleDeliveryRegistration {
	return &BundleDeliveryRegistration{
		failChan:               failChan,
		retryChan:              retryChan,
		lastSentBundleVersion:  status.NewBundleVersion(0, 0), // smallest version possible
		conditionSubscriptions: make(map[DeliveryEvent]map[DeliveryArgType]func(interface{}) bool),
		actionSubscriptions:    make(map[DeliveryEvent]map[DeliveryArgType][]func(interface{})),
		subscriptionsLock:      sync.Mutex{},
		generationLock:         sync.Mutex{},
	}
}

// BundleDeliveryRegistration abstract the registration for different bundle delivery information and functionalities
// in one place, according to bundle key in transport layer.
type BundleDeliveryRegistration struct {
	failChan               chan *error
	retryChan              chan *Message
	lastSentBundleVersion  *status.BundleVersion
	conditionSubscriptions map[DeliveryEvent]map[DeliveryArgType]func(interface{}) bool
	actionSubscriptions    map[DeliveryEvent]map[DeliveryArgType][]func(interface{})
	subscriptionsLock      sync.Mutex
	generationLock         sync.Mutex
}

// AddAction adds a delegate action to be called when a relevant delivery-event occurs with the given arg type.
func (bdr *BundleDeliveryRegistration) AddAction(eventType DeliveryEvent, argumentType DeliveryArgType,
	action func(interface{})) {
	bdr.subscriptionsLock.Lock()
	defer bdr.subscriptionsLock.Unlock()

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
	bdr.subscriptionsLock.Lock()
	defer bdr.subscriptionsLock.Unlock()

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
	arg interface{}, blocking bool) {
	bdr.subscriptionsLock.Lock()
	defer bdr.subscriptionsLock.Unlock()

	if actionsMap, eventFound := bdr.actionSubscriptions[eventType]; eventFound {
		for _, action := range actionsMap[argType] {
			if !blocking {
				go action(arg)
			} else {
				action(arg)
			}
		}
	}
}

// CheckEventCondition checks conditions on eventType that expect an argument of argType.
func (bdr *BundleDeliveryRegistration) CheckEventCondition(eventType DeliveryEvent, argType DeliveryArgType,
	arg interface{}) bool {
	bdr.subscriptionsLock.Lock()
	defer bdr.subscriptionsLock.Unlock()

	if conditionMap, eventFound := bdr.conditionSubscriptions[eventType]; eventFound {
		if condition, found := conditionMap[argType]; found {
			return condition(arg)
		}
	}
	// if no condition exists for this eventType / argType, assume no one cares.
	return true
}

// SetLastSentBundleVersion updates the registered bundle's last sent version.
func (bdr *BundleDeliveryRegistration) SetLastSentBundleVersion(version *status.BundleVersion) {
	bdr.generationLock.Lock()
	defer bdr.generationLock.Unlock()

	bdr.lastSentBundleVersion = version
}

// GetLastSentBundleVersion returns the registered bundle's last sent version.
func (bdr *BundleDeliveryRegistration) GetLastSentBundleVersion() *status.BundleVersion {
	bdr.generationLock.Lock()
	defer bdr.generationLock.Unlock()

	return bdr.lastSentBundleVersion
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
