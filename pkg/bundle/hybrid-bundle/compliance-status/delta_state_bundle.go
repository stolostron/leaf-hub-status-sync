package compliancestatus

import (
	"errors"
	"sync"

	v1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	statusbundle "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
	bundlepkg "github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	hybridbundle "github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle/hybrid-bundle"
)

const (
	unknownComplianceStatus = "unknown"
	unsentStatus            = "unsent"
)

var errPolicyStatusUnchanged = errors.New("policy status did not changed")

// NewDeltaComplianceStatusBundle creates a new instance of DeltaComplianceStatusBundle.
func NewDeltaComplianceStatusBundle(leafHubName string, baseBundle bundlepkg.Bundle,
	clustersPerPolicyBundle *bundlepkg.ClustersPerPolicyBundle, incarnation uint64,
	extractObjIDFunc bundlepkg.ExtractObjIDFunc) hybridbundle.DeltaStateBundle {
	return &DeltaComplianceStatusBundle{
		BaseDeltaComplianceStatusBundle: statusbundle.BaseDeltaComplianceStatusBundle{
			Objects:           make([]*statusbundle.PolicyGenericComplianceStatus, 0),
			LeafHubName:       leafHubName,
			BaseBundleVersion: baseBundle.GetBundleVersion(),
			BundleVersion:     statusbundle.NewBundleVersion(incarnation, 0),
		},
		baseBundle:                  baseBundle,
		clustersPerPolicyBaseBundle: clustersPerPolicyBundle,
		policyComplianceRecords:     make(map[string]*policyComplianceStatus),
		extractObjIDFunc:            extractObjIDFunc,
		lock:                        sync.Mutex{},
	}
}

// DeltaComplianceStatusBundle abstracts management of compliance status bundle.
type DeltaComplianceStatusBundle struct {
	statusbundle.BaseDeltaComplianceStatusBundle
	baseBundle                  bundlepkg.Bundle
	clustersPerPolicyBaseBundle *bundlepkg.ClustersPerPolicyBundle
	policyComplianceRecords     map[string]*policyComplianceStatus
	extractObjIDFunc            bundlepkg.ExtractObjIDFunc
	lock                        sync.Mutex
}

type policyComplianceStatus struct {
	compliantClustersMap    map[string]struct{}
	nonCompliantClustersMap map[string]struct{}
	unknownClustersMap      map[string]struct{}
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *DeltaComplianceStatusBundle) UpdateObject(object bundlepkg.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	policy, legal := object.(*v1.Policy)
	if !legal {
		return
	}

	originPolicyID, ok := bundle.extractObjIDFunc(object)
	if !ok {
		return // can't update the object without finding its id
	}

	// if policy is new then sync what's in the clustersPerPolicy base (already sent)
	if _, policyHasRecords := bundle.policyComplianceRecords[originPolicyID]; !policyHasRecords {
		bundle.updateSpecificPolicyRecordsFromBase(originPolicyID)
	}

	// get policy compliance status, this will also update info in records
	policyComplianceObject, err := bundle.getPolicyComplianceStatus(originPolicyID, policy)
	if err != nil {
		return // error found means object should be skipped
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle
		bundle.Objects = append(bundle.Objects, policyComplianceObject)
		bundle.BundleVersion.Generation++

		return
	}

	// object found, update content
	bundle.updatePolicyComplianceStatus(index, policyComplianceObject)
	bundle.BundleVersion.Generation++
}

// DeleteObject function to delete all instances of a single object inside a bundle.
func (bundle *DeltaComplianceStatusBundle) DeleteObject(object bundlepkg.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	originPolicyID, found := object.GetAnnotations()[datatypes.OriginOwnerReferenceAnnotation]
	if !found {
		return // origin owner reference annotation not found, cannot handle this policy
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}

	// do not increase generation, no need to send bundle when policy is removed (clusters per policy bundle is sent)
	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects

	// delete from policies records and the policies' existence map
	delete(bundle.policyComplianceRecords, originPolicyID)
}

// GetBundleVersion function to get bundle version.
func (bundle *DeltaComplianceStatusBundle) GetBundleVersion() *statusbundle.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.BundleVersion
}

// GetObjects function to return the hybrid bundle's objects.
func (bundle *DeltaComplianceStatusBundle) GetObjects() interface{} {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.Objects
}

// GetBundleType returns a pointer to the bundle key (changes based on mode).
func (bundle *DeltaComplianceStatusBundle) GetBundleType() string {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return datatypes.PolicyDeltaComplianceMsgKey
}

// SyncWithBase syncs the bundle with base.
func (bundle *DeltaComplianceStatusBundle) SyncWithBase() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	// update version
	bundle.BaseBundleVersion = bundle.baseBundle.GetBundleVersion()

	// update policy records from the ClustersPerPolicy bundle's (full-state) status
	bundle.updatePolicyRecordsFromBase()
}

// FlushObjects flushes the objects in the bundle (after delivery).
func (bundle *DeltaComplianceStatusBundle) FlushObjects() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.Objects = nil
}

func (bundle *DeltaComplianceStatusBundle) updateSpecificPolicyRecordsFromBase(policyID string) {
	policiesGenericComplianceStatuses := bundle.clustersPerPolicyBaseBundle.BaseClustersPerPolicyBundle.Objects

	for _, genericComplianceStatus := range policiesGenericComplianceStatuses {
		if genericComplianceStatus.PolicyID != policyID {
			continue
		}

		// create new records for policy
		bundle.policyComplianceRecords[policyID] = &policyComplianceStatus{
			compliantClustersMap:    make(map[string]struct{}),
			nonCompliantClustersMap: make(map[string]struct{}),
			unknownClustersMap:      make(map[string]struct{}),
		}

		// fill it up from base
		bundle.syncGenericStatus(genericComplianceStatus)
	}
}

// updateObjectRecordsFromBase ensures that the delta state objects records are up-to-date when bundle gets enabled.
func (bundle *DeltaComplianceStatusBundle) updatePolicyRecordsFromBase() {
	policiesGenericComplianceStatuses := bundle.clustersPerPolicyBaseBundle.BaseClustersPerPolicyBundle.Objects

	for _, genericComplianceStatus := range policiesGenericComplianceStatuses {
		// create new records for policy
		bundle.policyComplianceRecords[genericComplianceStatus.PolicyID] = &policyComplianceStatus{
			compliantClustersMap:    make(map[string]struct{}),
			nonCompliantClustersMap: make(map[string]struct{}),
			unknownClustersMap:      make(map[string]struct{}),
		}

		// fill it up from base
		bundle.syncGenericStatus(genericComplianceStatus)
	}
}

func (bundle *DeltaComplianceStatusBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}

	return -1, errObjectNotFound
}

// getPolicyComplianceStatus gets compliance statuses of a new policy object (relative to this bundle).
func (bundle *DeltaComplianceStatusBundle) getPolicyComplianceStatus(originPolicyID string,
	policy *v1.Policy) (*statusbundle.PolicyGenericComplianceStatus, error) {
	compliantClusters, nonCompliantClusters, unknownComplianceClusters := bundle.getChangedClusters(policy)

	if len(compliantClusters)+len(nonCompliantClusters)+len(unknownComplianceClusters) == 0 {
		return nil, errPolicyStatusUnchanged
	}

	return &statusbundle.PolicyGenericComplianceStatus{
		PolicyID:                  originPolicyID,
		CompliantClusters:         compliantClusters,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}, nil
}

// getChangedClusters returns arrays of changed compliance (cluster names).
func (bundle *DeltaComplianceStatusBundle) getChangedClusters(policy *v1.Policy) ([]string, []string, []string) {
	compliantClusters := make([]string, 0)
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)

	originPolicyID := policy.GetAnnotations()[datatypes.OriginOwnerReferenceAnnotation]

	for _, clusterCompliance := range policy.Status.Status {
		if bundle.getRecordedClusterComplianceStatus(originPolicyID, clusterCompliance.ClusterName) ==
			clusterCompliance.ComplianceState {
			// cluster compliance didn't change, skip
			continue
		}
		// add to bundle
		switch clusterCompliance.ComplianceState {
		case v1.Compliant:
			compliantClusters = append(compliantClusters, clusterCompliance.ClusterName)
		case v1.NonCompliant:
			nonCompliantClusters = append(nonCompliantClusters, clusterCompliance.ClusterName)
		default:
			unknownComplianceClusters = append(unknownComplianceClusters, clusterCompliance.ClusterName)
		}
	}

	// update recorded statuses
	bundle.updateRecordedClusterComplianceStatus(&originPolicyID, compliantClusters,
		nonCompliantClusters, unknownComplianceClusters)

	return compliantClusters, nonCompliantClusters, unknownComplianceClusters
}

// getRecordedClusterComplianceStatus gets the recorded compliance of a given cluster in a policy.
func (bundle *DeltaComplianceStatusBundle) getRecordedClusterComplianceStatus(originPolicyID string,
	clusterName string) v1.ComplianceState {
	policyRecords := bundle.policyComplianceRecords[originPolicyID]

	if _, clusterExistsInCompliantMap :=
		policyRecords.compliantClustersMap[clusterName]; clusterExistsInCompliantMap {
		return v1.Compliant
	}

	if _, clusterExistsInNonCompliantMap :=
		policyRecords.nonCompliantClustersMap[clusterName]; clusterExistsInNonCompliantMap {
		return v1.NonCompliant
	}

	if _, clusterExistsInUnknownMap :=
		policyRecords.unknownClustersMap[clusterName]; clusterExistsInUnknownMap {
		return unknownComplianceStatus
	}

	return unsentStatus
}

// updateRecordedClusterComplianceStatus updates the compliance records of a given policy.
func (bundle *DeltaComplianceStatusBundle) updateRecordedClusterComplianceStatus(originPolicyID *string,
	compliantClusters []string, nonCompliantClusters []string, unknownClusters []string) {
	policyRecords := bundle.policyComplianceRecords[*originPolicyID]

	for _, compliantCluster := range compliantClusters {
		policyRecords.compliantClustersMap[compliantCluster] = struct{}{}
		delete(policyRecords.nonCompliantClustersMap, compliantCluster)
		delete(policyRecords.unknownClustersMap, compliantCluster)
	}

	for _, nonCompliantCluster := range nonCompliantClusters {
		policyRecords.nonCompliantClustersMap[nonCompliantCluster] = struct{}{}
		delete(policyRecords.compliantClustersMap, nonCompliantCluster)
		delete(policyRecords.unknownClustersMap, nonCompliantCluster)
	}

	for _, unknownCluster := range unknownClusters {
		policyRecords.unknownClustersMap[unknownCluster] = struct{}{}
		delete(policyRecords.compliantClustersMap, unknownCluster)
		delete(policyRecords.nonCompliantClustersMap, unknownCluster)
	}
}

// updatePolicyComplianceStatus updates compliance statuses of an already listed policy object.
func (bundle *DeltaComplianceStatusBundle) updatePolicyComplianceStatus(policyIndex int,
	newPolicyStatus *statusbundle.PolicyGenericComplianceStatus) {
	// get existing policy state
	existingPolicyState := bundle.getExistingPolicyState(policyIndex)

	// update the policy state above
	for _, cluster := range newPolicyStatus.CompliantClusters {
		existingPolicyState[cluster] = v1.Compliant
	}

	for _, cluster := range newPolicyStatus.NonCompliantClusters {
		existingPolicyState[cluster] = v1.NonCompliant
	}

	for _, cluster := range newPolicyStatus.UnknownComplianceClusters {
		existingPolicyState[cluster] = unknownComplianceStatus
	}

	// generate new compliance lists from the updated policy state map
	compliantClusters := make([]string, 0)
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)

	for cluster, compliance := range existingPolicyState {
		switch compliance {
		case v1.Compliant:
			compliantClusters = append(compliantClusters, cluster)
		case v1.NonCompliant:
			nonCompliantClusters = append(nonCompliantClusters, cluster)
		default:
			unknownComplianceClusters = append(unknownComplianceClusters, cluster)
		}
	}

	// update policy
	bundle.Objects[policyIndex].CompliantClusters = compliantClusters
	bundle.Objects[policyIndex].NonCompliantClusters = nonCompliantClusters
	bundle.Objects[policyIndex].UnknownComplianceClusters = unknownComplianceClusters
}

// getExistingPolicyState returns a map of [cluster name -> compliance state] for the policy indexed by policyIndex.
func (bundle *DeltaComplianceStatusBundle) getExistingPolicyState(policyIndex int) map[string]v1.ComplianceState {
	existingPolicyState := make(map[string]v1.ComplianceState)

	for _, cluster := range bundle.Objects[policyIndex].CompliantClusters {
		existingPolicyState[cluster] = v1.Compliant
	}

	for _, cluster := range bundle.Objects[policyIndex].NonCompliantClusters {
		existingPolicyState[cluster] = v1.NonCompliant
	}

	for _, cluster := range bundle.Objects[policyIndex].UnknownComplianceClusters {
		existingPolicyState[cluster] = unknownComplianceStatus
	}

	return existingPolicyState
}

func (bundle *DeltaComplianceStatusBundle) syncGenericStatus(status *statusbundle.PolicyGenericComplianceStatus) {
	for _, compliantCluster := range status.CompliantClusters {
		bundle.policyComplianceRecords[status.PolicyID].compliantClustersMap[compliantCluster] = struct{}{}
	}

	for _, nonCompliantCluster := range status.NonCompliantClusters {
		bundle.policyComplianceRecords[status.PolicyID].nonCompliantClustersMap[nonCompliantCluster] = struct{}{}
	}

	for _, unknownCluster := range status.CompliantClusters {
		bundle.policyComplianceRecords[status.PolicyID].unknownClustersMap[unknownCluster] = struct{}{}
	}
}