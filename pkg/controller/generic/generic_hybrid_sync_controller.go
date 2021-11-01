package generic

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

type syncMode int

const (
	completeStateMode      syncMode = iota
	recoveryMode           syncMode = iota
	deltaStateMode         syncMode = iota
	internalChanBufferSize          = 42
)

// NewGenericHybridSyncController creates a new instance of GenericHybridStatusManager.
func NewGenericHybridSyncController(log logr.Logger, sentDeltaCountSwitchFactor int, failCountSwitchFactor int,
	completeStateBundle bundle.HybridBundle, deltaStateBundle bundle.DeltaStateBundle,
	completeStateBundleDeliveryRegistration *transport.BundleDeliveryRegistration,
	deltaStateBundleDeliveryRegistration *transport.BundleDeliveryRegistration,
	failChan chan *error) *HybridSyncController {
	return &HybridSyncController{
		log:                                     log,
		completeStateBundle:                     completeStateBundle,
		completeStateBundleDeliveryRegistration: completeStateBundleDeliveryRegistration,
		deltaStateBundle:                        deltaStateBundle,
		deltaStateBundleDeliveryRegistration:    deltaStateBundleDeliveryRegistration,
		failChan:                                failChan,
		sentDeltaChan:                           make(chan struct{}, internalChanBufferSize),
		failResetChan:                           make(chan struct{}, internalChanBufferSize),
		sentDeltaCountSwitchFactor:              sentDeltaCountSwitchFactor,
		failCountSwitchFactor:                   failCountSwitchFactor,
		mode:                                    completeStateMode,
		lock:                                    sync.Mutex{},
	}
}

// HybridSyncController manages two bundle collection entries, one per mode (complete/delta sync).
type HybridSyncController struct {
	log                                     logr.Logger
	completeStateBundle                     bundle.HybridBundle
	completeStateBundleDeliveryRegistration *transport.BundleDeliveryRegistration
	deltaStateBundle                        bundle.DeltaStateBundle
	deltaStateBundleDeliveryRegistration    *transport.BundleDeliveryRegistration
	failChan                                chan *error
	sentDeltaChan                           chan struct{}
	failResetChan                           chan struct{}
	sentDeltaCountSwitchFactor              int
	failCountSwitchFactor                   int
	mode                                    syncMode
	lock                                    sync.Mutex
}

// Start function starts hybrid status manager.
func (hsc *HybridSyncController) Start(stopChannel <-chan struct{}) error {
	ctx, cancelContext := context.WithCancel(context.Background())
	defer cancelContext()

	// add delivery events actions
	hsc.addOnDeliveryAttemptAction()
	// add delivery conditions
	hsc.addOnDeliveryPreAttemptConditions()

	// enable the complete state bundle (hybrid bundles are disabled by default)
	hsc.completeStateBundle.Enable()
	hsc.deltaStateBundle.Enable()

	go hsc.manageState(ctx)

	for {
		<-stopChannel // blocking wait until getting stop event on the stop channel
		cancelContext()
		close(hsc.sentDeltaChan)
		close(hsc.failResetChan)

		return nil
	}
}

func (hsc *HybridSyncController) addOnRecoverySuccessAction() {
	// if a complete state bundle gets shipped successfully then we should reset fail count. if it's a delta bundle-
	// then we should not care, success of delta-bundle transportation (including retries) does not reduce fail count
	// reminder: actions are invoked asynchronously
	hsc.completeStateBundleDeliveryRegistration.AddAction(transport.DeliverySuccess, transport.ArgTypeNone,
		func(interface{}) {
			hsc.lock.Lock()
			defer hsc.lock.Unlock()

			if hsc.mode == recoveryMode {
				hsc.failResetChan <- struct{}{}

				hsc.mode = deltaStateMode
				hsc.deltaStateBundle.Enable()
			}
		})
}

func (hsc *HybridSyncController) addOnDeliveryAttemptAction() {
	afterDeliveryAttemptAction := func(sentObjectsCount interface{}) {
		hsc.lock.Lock()
		defer hsc.lock.Unlock()

		if hsc.mode == completeStateMode {
			// means a complete-state bundle was sent, should switch mode now and update delta-state bundle's base
			hsc.log.Info("switching to delta-state mode")
			hsc.mode = deltaStateMode
			hsc.deltaStateBundle.Enable()
		} else {
			// delta bundle was sent, remove the transported objects
			hsc.deltaStateBundle.FlushOrderedObjects(sentObjectsCount.(int))
			// let mode manager routine know
			go func() {
				// separate routine to write to channel due to locks
				hsc.sentDeltaChan <- struct{}{}
			}()
		}
	}

	hsc.deltaStateBundleDeliveryRegistration.AddAction(transport.AfterDeliveryAttempt,
		transport.ArgTypeBundleObjectsCount, afterDeliveryAttemptAction)

	if hsc.sentDeltaCountSwitchFactor > 0 { // add switch action only if wanted
		hsc.completeStateBundleDeliveryRegistration.AddAction(transport.AfterDeliveryAttempt,
			transport.ArgTypeBundleObjectsCount, afterDeliveryAttemptAction)
	}
}

func (hsc *HybridSyncController) addOnDeliveryPreAttemptConditions() {
	// add condition that allows deltas to be transported only when deltaStateMode is active
	deltaStateDeliveryPreAttemptCondition := func(interface{}) bool {
		hsc.lock.Lock()
		defer hsc.lock.Unlock()

		return hsc.mode == deltaStateMode
	}
	// add condition that allows complete-states to be transported only when completeStateMode is active or if we-
	// are in recovery mode (disconnection)
	completeStateDeliveryPreAttemptCondition := func(interface{}) bool {
		hsc.lock.Lock()
		defer hsc.lock.Unlock()

		return hsc.mode == completeStateMode || hsc.mode == recoveryMode
	}

	hsc.completeStateBundleDeliveryRegistration.AddCondition(transport.BeforeDeliveryAttempt,
		transport.ArgTypeNone, completeStateDeliveryPreAttemptCondition)
	hsc.deltaStateBundleDeliveryRegistration.AddCondition(transport.BeforeDeliveryAttempt,
		transport.ArgTypeNone, deltaStateDeliveryPreAttemptCondition)

	hsc.deltaStateBundleDeliveryRegistration.AddCondition(transport.BeforeDeliveryRetry,
		transport.ArgTypeNone, deltaStateDeliveryPreAttemptCondition) // if mode switches, don't retry hanging deltas
}

func (hsc *HybridSyncController) manageState(ctx context.Context) {
	failCount := 0
	sentDeltaCount := 0

	hsc.log.Info("started state management")

	for {
		select {
		case <-ctx.Done():
			return

		case <-hsc.failResetChan:
			failCount = 0

		case <-hsc.failChan:
			failCount++
			if failCount == hsc.failCountSwitchFactor {
				hsc.lock.Lock()

				// switch mode and disable (flush) delta bundle
				hsc.log.Info("switching to recovery mode")
				hsc.mode = recoveryMode
				hsc.completeStateBundle.Enable()
				hsc.deltaStateBundle.Disable()

				hsc.lock.Unlock()
			}

		case <-hsc.sentDeltaChan:
			sentDeltaCount++
			if sentDeltaCount >= hsc.sentDeltaCountSwitchFactor {
				hsc.lock.Lock()

				// switch mode and disable (flush) delta bundle
				hsc.log.Info("switching to complete-state mode")
				hsc.mode = completeStateMode
				hsc.completeStateBundle.Enable()
				hsc.deltaStateBundle.Disable()

				sentDeltaCount = 0
				hsc.lock.Unlock()
			}
		}
	}
}
