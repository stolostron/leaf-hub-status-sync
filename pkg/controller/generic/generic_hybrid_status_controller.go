package generic

import (
	"context"
	"sync"

	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
)

const (
	completeStateMode = iota
	deltaStateMode
)

// NewGenericHybridStatusController creates a new instance of GenericHybridStatusManager.
func NewGenericHybridStatusController(completeStateBundle bundle.HybridBundle,
	deltaStateBundle bundle.DeltaStateBundle) *HybridStatusController {
	return &HybridStatusController{
		completeStateBundle: completeStateBundle,
		deltaStateBundle:    deltaStateBundle,
		mode:                completeStateMode,
		lock:                sync.Mutex{},
	}
}

// HybridStatusController manages two bundle collection entries, one per mode (complete/delta sync).
type HybridStatusController struct {
	completeStateBundle bundle.HybridBundle
	deltaStateBundle    bundle.DeltaStateBundle
	mode                int
	lock                sync.Mutex
}

// Start function starts hybrid status manager.
func (hsc *HybridStatusController) Start(stopChannel <-chan struct{}) error {
	ctx, cancelContext := context.WithCancel(context.Background())
	defer cancelContext()

	// enable the complete state bundle (hybrid bundles are disabled by default)
	hsc.completeStateBundle.Enable()

	go hsc.manageState(ctx)

	for {
		<-stopChannel // blocking wait until getting stop event on the stop channel.
		cancelContext()

		return nil
	}
}

// GenerateDeliveryConsumptionFunc returns a function that can refer to the manager to handle mode switching.
func (hsc *HybridStatusController) GenerateDeliveryConsumptionFunc() func(int) {
	// returned function will be called whenever a watched collection-entry is sent via transport.
	return func(sentObjectsCount int) {
		hsc.lock.Lock()
		defer hsc.lock.Unlock()

		if hsc.mode == completeStateMode {
			// means a complete-state bundle was sent, should switch mode now and update delta-state bundle's base
			hsc.mode = deltaStateMode
			hsc.deltaStateBundle.Enable()
			hsc.completeStateBundle.Disable()
		} else {
			// delta bundle was sent, remove the transported objects
			hsc.deltaStateBundle.FlushOrderedObjects(sentObjectsCount)
		}
	}
}

// GenerateCompleteStateBundlePredicate expands the given predicate with a check on if current mode is complete-state.
func (hsc *HybridStatusController) GenerateCompleteStateBundlePredicate(existingPred func() bool) func() bool {
	return func() bool {
		hsc.lock.Lock()
		defer hsc.lock.Unlock()

		return hsc.mode == completeStateMode && existingPred()
	}
}

// GenerateDeltaStateBundlePredicate expands the given predicate with a check on if current mode is delta-state.
func (hsc *HybridStatusController) GenerateDeltaStateBundlePredicate(existingPred func() bool) func() bool {
	return func() bool {
		hsc.lock.Lock()
		defer hsc.lock.Unlock()

		return hsc.mode == deltaStateMode && existingPred()
	}
}

func (hsc *HybridStatusController) manageState(ctx context.Context) {
	// TODO: implement mode (disconnections) handler
	for {
		<-ctx.Done()

		return
	}
}
