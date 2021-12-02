package generic

import (
	"errors"
	"sync"

	"github.com/go-logr/logr"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

type syncMode int8

const (
	completeStateMode syncMode = iota
	deltaStateMode    syncMode = iota
)

var errExpectingDeltaStateBundle = errors.New("expecting a BundleCollectionEntry that wraps a DeltaStateBundle bundle")

// NewGenericHybridSyncManager creates a manager that manages two BundleCollectionEntry instances that wrap a
// complete-state bundle and a delta-state bundle.
func NewGenericHybridSyncManager(log logr.Logger, transportObj transport.Transport,
	completeStateBundleCollectionEntry *BundleCollectionEntry, deltaStateBundleCollectionEntry *BundleCollectionEntry,
	sentDeltaCountSwitchFactor int) error {
	if sentDeltaCountSwitchFactor <= 0 {
		return nil // hybrid mode is not active, don't do anything.
	}
	// check that the delta state collection does indeed wrap a delta bundle
	deltaStateBundle, ok := deltaStateBundleCollectionEntry.bundle.(bundle.DeltaStateBundle)
	if !ok {
		return errExpectingDeltaStateBundle
	}

	genericHybridSyncManager := &hybridSyncManager{
		log:      log,
		syncMode: completeStateMode,
		bundleCollectionEntryMap: map[syncMode]*BundleCollectionEntry{
			completeStateMode: completeStateBundleCollectionEntry,
			deltaStateMode:    deltaStateBundleCollectionEntry,
		},
		deltaStateBundle:           deltaStateBundle,
		sentDeltaCountSwitchFactor: sentDeltaCountSwitchFactor,
		sentDeltaCount:             0,
		lock:                       sync.Mutex{},
	}

	genericHybridSyncManager.appendPredicates()
	genericHybridSyncManager.setCallbacks(transportObj)

	return nil
}

// hybridSyncManager manages two BundleCollectionEntry instances in application of hybrid-sync mode.
// won't get GC'd since callbacks are used.
type hybridSyncManager struct {
	log                        logr.Logger
	syncMode                   syncMode
	bundleCollectionEntryMap   map[syncMode]*BundleCollectionEntry
	deltaStateBundle           bundle.DeltaStateBundle
	sentDeltaCountSwitchFactor int
	sentDeltaCount             int
	lock                       sync.Mutex
}

func (manager *hybridSyncManager) appendPredicates() {
	// append predicates for mode-management
	for syncMode, bundleCollectionEntry := range manager.bundleCollectionEntryMap {
		entry := bundleCollectionEntry       // to use in func
		syncMode := syncMode                 // to use in func
		originalPredicate := entry.predicate // avoid recursion
		entry.predicate = func() bool {
			manager.lock.Lock()
			defer manager.lock.Unlock()

			return originalPredicate() && manager.syncMode == syncMode
		}
	}
}

func (manager *hybridSyncManager) setCallbacks(transportObj transport.Transport) {
	for _, bundleCollectionEntry := range manager.bundleCollectionEntryMap {
		transportObj.Subscribe(bundleCollectionEntry.transportBundleKey,
			map[transport.EventType]transport.EventCallback{
				transport.DeliveryAttempt: manager.handleTransportationAttempt,
				transport.DeliverySuccess: manager.handleTransportationSuccess,
				transport.DeliveryFailure: manager.handleTransportationFailure,
			})
	}
}

func (manager *hybridSyncManager) handleTransportationAttempt() {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	if manager.syncMode == completeStateMode {
		manager.switchToDeltaStateMode()
		return
	}

	// else we're in delta
	manager.sentDeltaCount++

	if manager.sentDeltaCount == manager.sentDeltaCountSwitchFactor {
		manager.switchToCompleteStateMode()
		return
	}

	// reset delta bundle objects
	manager.deltaStateBundle.Reset()
}

func (manager *hybridSyncManager) handleTransportationSuccess() {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	if manager.syncMode == deltaStateMode {
		return
	}

	manager.switchToDeltaStateMode()
}

func (manager *hybridSyncManager) handleTransportationFailure() {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	if manager.syncMode == completeStateMode {
		return
	}

	manager.switchToCompleteStateMode()
}

func (manager *hybridSyncManager) switchToCompleteStateMode() {
	manager.syncMode = completeStateMode
}

func (manager *hybridSyncManager) switchToDeltaStateMode() {
	manager.syncMode = deltaStateMode
	manager.sentDeltaCount = 0

	manager.deltaStateBundle.Reset()
	manager.deltaStateBundle.SyncState()
}
