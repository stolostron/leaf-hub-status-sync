package generic

import (
	"errors"
	"sync"

	"github.com/go-logr/logr"
	"github.com/stolostron/hub-of-hubs-data-types/bundle/status"
	"github.com/stolostron/leaf-hub-status-sync/pkg/bundle"
	"github.com/stolostron/leaf-hub-status-sync/pkg/transport"
)

var errExpectingDeltaStateBundle = errors.New("expecting a BundleCollectionEntry that wraps a DeltaStateBundle bundle")

// NewHybridSyncManager creates a manager that manages two BundleCollectionEntry instances that wrap a
// complete-state bundle and a delta-state bundle.
func NewHybridSyncManager(log logr.Logger, transportObj transport.Transport,
	completeStateBundleCollectionEntry *BundleCollectionEntry, deltaStateBundleCollectionEntry *BundleCollectionEntry,
	sentDeltaCountSwitchFactor int) error {
	// check that the delta state collection does indeed wrap a delta bundle
	deltaStateBundle, ok := deltaStateBundleCollectionEntry.bundle.(bundle.DeltaStateBundle)
	if !ok {
		return errExpectingDeltaStateBundle
	}

	hybridSyncManager := &hybridSyncManager{
		log:            log,
		activeSyncMode: status.CompleteStateMode,
		bundleCollectionEntryMap: map[status.BundleSyncMode]*BundleCollectionEntry{
			status.CompleteStateMode: completeStateBundleCollectionEntry,
			status.DeltaStateMode:    deltaStateBundleCollectionEntry,
		},
		deltaStateBundle:           deltaStateBundle,
		sentDeltaCountSwitchFactor: sentDeltaCountSwitchFactor,
		sentDeltaCount:             0,
		lock:                       sync.Mutex{},
	}

	hybridSyncManager.appendPredicates()

	if hybridSyncManager.isEnabled(transportObj) { // hybrid mode may be disabled in some different scenarios.
		hybridSyncManager.setCallbacks(transportObj)
	}

	return nil
}

// hybridSyncManager manages two BundleCollectionEntry instances in application of hybrid-sync mode.
// won't get collected by the GC since callbacks are used.
type hybridSyncManager struct {
	log                        logr.Logger
	activeSyncMode             status.BundleSyncMode
	bundleCollectionEntryMap   map[status.BundleSyncMode]*BundleCollectionEntry
	deltaStateBundle           bundle.DeltaStateBundle
	sentDeltaCountSwitchFactor int
	sentDeltaCount             int
	lock                       sync.Mutex
}

func (manager *hybridSyncManager) appendPredicates() {
	// append predicates for mode-management
	for syncMode, bundleCollectionEntry := range manager.bundleCollectionEntryMap {
		entry := bundleCollectionEntry       // to use in func
		mode := syncMode                     // to use in func
		originalPredicate := entry.predicate // avoid recursion
		entry.predicate = func() bool {
			manager.lock.Lock()
			defer manager.lock.Unlock()

			return manager.activeSyncMode == mode && originalPredicate()
		}
	}
}

func (manager *hybridSyncManager) isEnabled(transportObj transport.Transport) bool {
	if manager.sentDeltaCountSwitchFactor <= 0 || !transportObj.SupportsDeltaBundles() {
		return false
	}

	return true
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

	if manager.activeSyncMode == status.CompleteStateMode {
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

	if manager.activeSyncMode == status.DeltaStateMode {
		return
	}

	manager.switchToDeltaStateMode()
}

func (manager *hybridSyncManager) handleTransportationFailure() {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	if manager.activeSyncMode == status.CompleteStateMode {
		return
	}

	manager.log.Info("transportation failure callback invoked")
	manager.switchToCompleteStateMode()
}

func (manager *hybridSyncManager) switchToCompleteStateMode() {
	manager.log.Info("switched to complete-state mode")
	manager.activeSyncMode = status.CompleteStateMode
}

func (manager *hybridSyncManager) switchToDeltaStateMode() {
	manager.log.Info("switched to delta-state mode")

	manager.activeSyncMode = status.DeltaStateMode
	manager.sentDeltaCount = 0

	manager.deltaStateBundle.Reset()
	manager.deltaStateBundle.SyncState()
}
