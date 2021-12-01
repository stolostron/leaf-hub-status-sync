package retryhandler

import (
	"context"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

// NewSyncRetryHandler returns a new instance of syncRetryHandler (exponential-backoff based).
func NewSyncRetryHandler(retryOperation func(interface{})) *SyncRetryHandler {
	return &SyncRetryHandler{
		retryOperation: retryOperation,
		operationArg:   nil,
		backOff:        backoff.NewExponentialBackOff(),
		evaluationChan: make(chan struct{}),
		resetChan:      make(chan struct{}),
		lock:           sync.Mutex{},
	}
}

// SyncRetryHandler executes a fixed retryOperation with a changeable argument based on exponential-backoff timing.
type SyncRetryHandler struct {
	retryOperation func(interface{})
	operationArg   interface{}
	backOff        backoff.BackOff
	evaluationChan chan struct{}
	resetChan      chan struct{}
	lock           sync.Mutex
}

// Start starts the handler (no operation is performed until at least one HandleFailure() is called).
func (handler *SyncRetryHandler) Start(ctx context.Context,
	deliveryRegistration *transport.BundleDeliveryRegistration) *SyncRetryHandler {
	// add reset action
	deliveryRegistration.AddAction(transport.DeliverySuccess, transport.ArgTypeNone,
		func(interface{}) {
			handler.resetChan <- struct{}{}
		})

	go handler.handleRetries(ctx)

	return handler
}

// HandleFailure receives an operationArg that fits the retryOperation passed when this handler was created and
// updates exponential backoff intervals if needed.
func (handler *SyncRetryHandler) HandleFailure(operationArg interface{}) {
	handler.lock.Lock()
	defer handler.lock.Unlock()

	handler.operationArg = operationArg
	handler.evaluationChan <- struct{}{}
}

func (handler *SyncRetryHandler) handleRetries(ctx context.Context) {
	timer := time.NewTimer(0)
	<-timer.C

	timerIsActive := false

	for {
		select {
		case <-ctx.Done():
			close(handler.evaluationChan)
			close(handler.resetChan)

			return

		case <-handler.evaluationChan:
			// a bundle that this handler handles failed, update timer only if not running
			timer = handler.updateTimer(timer, &timerIsActive)

		case <-handler.resetChan:
			// a successful sync occurred, reset
			timerIsActive = false

			timer.Stop()
			handler.backOff.Reset()

		case <-timer.C:
			// execute sync operation
			handler.executeOperation(&timerIsActive)
		}
	}
}

func (handler *SyncRetryHandler) updateTimer(currentTimer *time.Timer, timerIsActive *bool) *time.Timer {
	if *timerIsActive {
		return currentTimer // when the timer ticks it will retry the latest bundle
	}

	*timerIsActive = true

	return time.NewTimer(handler.backOff.NextBackOff())
}

func (handler *SyncRetryHandler) executeOperation(timerIsActive *bool) {
	handler.lock.Lock()
	defer handler.lock.Unlock()

	*timerIsActive = false

	handler.retryOperation(handler.operationArg)
}
