package compliancestatus

import (
	"sync"

	statusbundle "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
	bundlepkg "github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	hybridbundle "github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle/hybrid-bundle"
)

const (
	completeStateMode = iota
	deltaStateMode    = iota
	recoveryMode      = iota
)

// NewHybridComplianceStatusBundle creates a new instance of ComplianceStatusBundle.
func NewHybridComplianceStatusBundle(leafHubName string, clustersPerPolicyBundle *bundlepkg.ClustersPerPolicyBundle,
	deltasSentCountSwitchFactor int, incarnation uint64,
	extractObjIDFunc bundlepkg.ExtractObjIDFunc) hybridbundle.HybridBundle {
	completeStateBundle := NewCompleteComplianceStatusBundle(leafHubName, clustersPerPolicyBundle, incarnation,
		extractObjIDFunc)
	deltaStateBundle := NewDeltaComplianceStatusBundle(leafHubName, completeStateBundle, clustersPerPolicyBundle,
		incarnation, extractObjIDFunc)

	return &HybridComplianceStatusBundle{
		ExportedBundle:      completeStateBundle,
		syncMode:            completeStateMode,
		completeStateBundle: completeStateBundle,
		deltaStateBundle:    deltaStateBundle,
		lock:                sync.Mutex{},

		sentDeltasCount:        0,
		sentDeltasSwitchFactor: deltasSentCountSwitchFactor,
	}
}

// HybridComplianceStatusBundle abstracts management of compliance status bundle.
type HybridComplianceStatusBundle struct {
	ExportedBundle bundlepkg.Bundle // the bundle that gets marshalled when sending this hybrid bundle as payload

	syncMode            int
	completeStateBundle hybridbundle.CompleteStateBundle
	deltaStateBundle    hybridbundle.DeltaStateBundle
	lock                sync.Mutex

	sentDeltasCount        int
	sentDeltasSwitchFactor int
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *HybridComplianceStatusBundle) UpdateObject(object bundlepkg.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	if bundle.syncMode == deltaStateMode {
		bundle.deltaStateBundle.UpdateObject(object)
		return
	}

	bundle.completeStateBundle.UpdateObject(object)
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *HybridComplianceStatusBundle) DeleteObject(object bundlepkg.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	if bundle.syncMode == deltaStateMode {
		bundle.deltaStateBundle.DeleteObject(object)
		return
	}

	bundle.completeStateBundle.DeleteObject(object)
}

// GetBundleVersion function to get bundle version.
func (bundle *HybridComplianceStatusBundle) GetBundleVersion() *statusbundle.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	if bundle.syncMode == deltaStateMode {
		return bundle.deltaStateBundle.GetBundleVersion()
	}

	return bundle.completeStateBundle.GetBundleVersion()
}

// GetBundleType returns a pointer to the bundle key (changes based on mode).
func (bundle *HybridComplianceStatusBundle) GetBundleType() string {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	if bundle.syncMode == deltaStateMode {
		return bundle.deltaStateBundle.GetBundleType()
	}

	return bundle.completeStateBundle.GetBundleType()
}

// HandleBundleTransportationAttempt lets the bundle know that it was transported (attempted).
func (bundle *HybridComplianceStatusBundle) HandleBundleTransportationAttempt() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	if bundle.syncMode == deltaStateMode {
		if bundle.sentDeltasCount == bundle.sentDeltasSwitchFactor {
			// reached switch factor, switch to complete state mode
			bundle.syncMode = completeStateMode
			bundle.ExportedBundle = bundle.completeStateBundle

			return
		}

		bundle.deltaStateBundle.FlushObjects()
		bundle.sentDeltasSwitchFactor++

		return
	} else if bundle.syncMode == completeStateMode {
		// switch to deltaStateMode
		bundle.syncMode = deltaStateMode
		bundle.ExportedBundle = bundle.deltaStateBundle
		bundle.deltaStateBundle.SyncWithBase()
		// reset counter
		bundle.sentDeltasCount = 0
	}
}

// HandleTransportationSuccess lets the bundle know that the latest transportation failed.
func (bundle *HybridComplianceStatusBundle) HandleTransportationSuccess() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	if bundle.syncMode != recoveryMode {
		return
	}

	// switch to deltaStateMode
	bundle.syncMode = deltaStateMode
	bundle.ExportedBundle = bundle.deltaStateBundle
	bundle.deltaStateBundle.SyncWithBase()
}

// HandleTransportationFailure lets the bundle know that the latest transportation failed.
func (bundle *HybridComplianceStatusBundle) HandleTransportationFailure() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	// switch to recovery mode
	bundle.syncMode = recoveryMode
	bundle.ExportedBundle = bundle.completeStateBundle
}
