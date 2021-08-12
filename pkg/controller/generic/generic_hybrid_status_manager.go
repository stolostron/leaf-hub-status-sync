package generic

import (
	"sync"

	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
)

const (
	completeStateMode = iota
	deltaStateMode
)

// NewGenericHybridStatusManager creates a new instance of GenericHybridStatusManager.
func NewGenericHybridStatusManager(completeStateBundle bundle.HybridBundle,
	deltaStateBundle bundle.HybridBundle) *HybridStatusManager {
	return &HybridStatusManager{
		completeStateBundle: completeStateBundle,
		deltaStateBundle:    deltaStateBundle,
		mode:                completeStateMode,
		lock:                sync.Mutex{},
		stopChan:            make(chan struct{}, 1),
	}
}

// HybridStatusManager manages two bundle collection entries, one per mode (complete/delta sync).
type HybridStatusManager struct {
	completeStateBundle bundle.HybridBundle
	deltaStateBundle    bundle.HybridBundle
	mode                int
	lock                sync.Mutex
	startOnce           sync.Once
	stopOnce            sync.Once
	stopChan            chan struct{}
}

// Start function starts hybrid status manager.
func (hsm *HybridStatusManager) Start() {
	hsm.startOnce.Do(func() {
		// enable the complete state bundle (hybrid bundles are disabled by default)
		hsm.completeStateBundle.Enable()
		go hsm.manageState()
	})
}

// Stop function stops hybrid status manager.
func (hsm *HybridStatusManager) Stop() {
	hsm.stopOnce.Do(func() {
		hsm.stopChan <- struct{}{}
		close(hsm.stopChan)
	})
}

// GenerateDeliveryConsumptionFunc returns a function that can refer to the manager to handle mode switching.
func (hsm *HybridStatusManager) GenerateDeliveryConsumptionFunc() func(int) {
	// returned function will be called whenever a watched collection-entry is sent via transport.
	return func(sentObjectsCount int) {
		hsm.lock.Lock()
		defer hsm.lock.Unlock()

		if hsm.mode == completeStateMode {
			// means a complete-state bundle was sent, should switch mode now and update delta-state bundle's base
			hsm.mode = deltaStateMode
			hsm.deltaStateBundle.Enable()
		} else {
			// delta bundle was sent, remove the transported objects
			hsm.deltaStateBundle.DeleteOrderedObjects(sentObjectsCount)
		}
	}
}

// GenerateCompleteStateBundlePredicate expands the given predicate with a check on if current mode is complete-state.
func (hsm *HybridStatusManager) GenerateCompleteStateBundlePredicate(existingPred func() bool) func() bool {
	return func() bool {
		hsm.lock.Lock()
		defer hsm.lock.Unlock()

		return hsm.mode == completeStateMode && existingPred()
	}
}

// GenerateDeltaStateBundlePredicate expands the given predicate with a check on if current mode is delta-state.
func (hsm *HybridStatusManager) GenerateDeltaStateBundlePredicate(existingPred func() bool) func() bool {
	return func() bool {
		hsm.lock.Lock()
		defer hsm.lock.Unlock()

		return hsm.mode == deltaStateMode && existingPred()
	}
}

func (hsm *HybridStatusManager) manageState() {
	// TODO: implement mode (disconnections) handler
	for {
		<-hsm.stopChan

		return
	}
}
