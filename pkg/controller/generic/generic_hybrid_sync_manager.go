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

// NewGenericHybridSyncManager returns a new instance of HybridSyncManager.
func NewGenericHybridSyncManager(log logr.Logger, transportObj transport.Transport,
	completeStateBundleCollectionEntry *BundleCollectionEntry, deltaStateBundleCollectionEntry *BundleCollectionEntry,
	sentDeltaCountSwitchFactor int) (*HybridSyncManager, error) {
	// check that the delta state collection does indeed wrap a delta bundle
	deltaStateBundle, ok := deltaStateBundleCollectionEntry.bundle.(bundle.DeltaStateBundle)
	if !ok {
		return nil, errExpectingDeltaStateBundle
	}

	genericHybridSyncManager := &HybridSyncManager{
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

	return genericHybridSyncManager, nil
}

// HybridSyncManager manages two BundleCollectionEntry instances in application of hybrid-sync mode.
type HybridSyncManager struct {
	log                        logr.Logger
	syncMode                   syncMode
	bundleCollectionEntryMap   map[syncMode]*BundleCollectionEntry
	deltaStateBundle           bundle.DeltaStateBundle
	sentDeltaCountSwitchFactor int
	sentDeltaCount             int
	lock                       sync.Mutex
}

func (manager *HybridSyncManager) appendPredicates() {
	// append predicates for mode-management
	for syncMode, bundleCollectionEntry := range manager.bundleCollectionEntryMap {
		entry := bundleCollectionEntry // to use in func
		syncMode := syncMode           // to use in func
		entry.predicate = func() bool {
			manager.lock.Lock()
			defer manager.lock.Unlock()

			return entry.predicate() && manager.syncMode == syncMode
		}
	}
}

func (manager *HybridSyncManager) setCallbacks(transportObj transport.Transport) {
	for _, bundleCollectionEntry := range manager.bundleCollectionEntryMap {
		transportObj.Subscribe(bundleCollectionEntry.transportBundleKey,
			map[transport.EventType]transport.EventCallback{
				transport.DeliverySuccess: manager.handleTransportationSuccess,
				transport.DeliveryFailure: manager.handleTransportationFailure,
			})
	}
}

func (manager *HybridSyncManager) handleTransportationSuccess() {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	if manager.sentDeltaCountSwitchFactor == 0 {
		return // if switch factor is 0 then we only want complete state mode enabled
	}

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

func (manager *HybridSyncManager) handleTransportationFailure() {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	if manager.syncMode == completeStateMode {
		return
	}

	manager.switchToCompleteStateMode()
}

func (manager *HybridSyncManager) switchToCompleteStateMode() {
	manager.syncMode = completeStateMode
}

func (manager *HybridSyncManager) switchToDeltaStateMode() {
	manager.syncMode = deltaStateMode
	manager.sentDeltaCount = 0

	manager.deltaStateBundle.SyncState()
}
