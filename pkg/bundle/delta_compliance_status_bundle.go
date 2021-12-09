package bundle

import (
	"errors"
	"sync"

	set "github.com/deckarep/golang-set"
	v1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	statusbundle "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
)

const (
	deltaComplianceStatusBundleID = datatypes.PolicyDeltaComplianceMsgKey
	unknownComplianceStatus       = "unknown"
)

var errPolicyStatusUnchanged = errors.New("policy status did not changed")

// NewDeltaComplianceStatusBundle creates a new instance of DeltaComplianceStatusBundle.
func NewDeltaComplianceStatusBundle(leafHubName string, baseBundle Bundle,
	clustersPerPolicyBundle *ClustersPerPolicyBundle, incarnation uint64,
	extractObjIDFunc ExtractObjIDFunc) DeltaStateBundle {
	return &DeltaComplianceStatusBundle{
		BaseDeltaComplianceStatusBundle: statusbundle.BaseDeltaComplianceStatusBundle{
			Objects:           make([]*statusbundle.PolicyGenericComplianceStatus, 0),
			LeafHubName:       leafHubName,
			BaseBundleVersion: statusbundle.NewBundleVersion(incarnation, baseBundle.GetBundleVersion().Generation),
			BundleVersion:     statusbundle.NewBundleVersion(incarnation, 0),
		},
		baseBundle:                  baseBundle,
		clustersPerPolicyBaseBundle: clustersPerPolicyBundle,
		complianceRecordsCache:      make(map[string]*policyComplianceStatus),
		extractObjIDFunc:            extractObjIDFunc,
		lock:                        sync.Mutex{},
	}
}

// DeltaComplianceStatusBundle abstracts management of compliance status bundle.
type DeltaComplianceStatusBundle struct {
	statusbundle.BaseDeltaComplianceStatusBundle
	baseBundle                  Bundle
	clustersPerPolicyBaseBundle *ClustersPerPolicyBundle
	complianceRecordsCache      map[string]*policyComplianceStatus
	extractObjIDFunc            ExtractObjIDFunc
	lock                        sync.Mutex
}

// policyComplianceStatus holds each policy's full compliance status at all times. It is needed since the
// clustersPerPolicy bundle is updated first (sending order dependency), therefore we cannot use its generic statuses.
type policyComplianceStatus struct {
	compliantClustersSet    set.Set
	nonCompliantClustersSet set.Set
	unknownClustersSet      set.Set
}

// GetID returns type identifier for this bundle.
func (bundle *DeltaComplianceStatusBundle) GetID() string {
	return deltaComplianceStatusBundleID
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *DeltaComplianceStatusBundle) UpdateObject(object Object) {
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

	// if policy is new then sync what's in the clustersPerPolicy base
	if _, policyHasRecords := bundle.complianceRecordsCache[originPolicyID]; !policyHasRecords {
		bundle.updateSpecificPolicyRecordsFromBase(originPolicyID)
		return
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
func (bundle *DeltaComplianceStatusBundle) DeleteObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	originPolicyID, ok := bundle.extractObjIDFunc(object)
	if !ok {
		return // cant update the object without finding its id.
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}

	// do not increase generation, no need to send bundle when policy is removed (clusters per policy bundle is sent)
	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects

	// delete from policies records and the policies' existence map
	delete(bundle.complianceRecordsCache, originPolicyID)
}

// GetBundleVersion function to get bundle version.
func (bundle *DeltaComplianceStatusBundle) GetBundleVersion() *statusbundle.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.BundleVersion
}

// SyncState syncs the state of the delta-bundle with the full-state.
func (bundle *DeltaComplianceStatusBundle) SyncState() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	// update version
	bundle.BaseBundleVersion.Generation = bundle.baseBundle.GetBundleVersion().Generation

	// update policy records from the ClustersPerPolicy bundle's (full-state) status
	bundle.complianceRecordsCache = make(map[string]*policyComplianceStatus)
	bundle.updatePolicyRecordsFromBase()
}

// Reset flushes the objects in the bundle (after delivery).
func (bundle *DeltaComplianceStatusBundle) Reset() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.Objects = nil // safe since go1.0
}

func (bundle *DeltaComplianceStatusBundle) updateSpecificPolicyRecordsFromBase(policyID string) {
	policiesGenericComplianceStatuses := bundle.clustersPerPolicyBaseBundle.BaseClustersPerPolicyBundle.Objects

	for _, genericComplianceStatus := range policiesGenericComplianceStatuses {
		if genericComplianceStatus.PolicyID != policyID {
			continue
		}

		// create new records for policy and fill it with status
		bundle.syncGenericStatus(genericComplianceStatus)

		break // found the policy, no need to continue
	}
}

// updateObjectRecordsFromBase ensures that the delta state objects records are up-to-date when bundle gets enabled.
func (bundle *DeltaComplianceStatusBundle) updatePolicyRecordsFromBase() {
	policiesGenericComplianceStatuses := bundle.clustersPerPolicyBaseBundle.BaseClustersPerPolicyBundle.Objects

	for _, genericComplianceStatus := range policiesGenericComplianceStatuses {
		// create new records for policy and fill it with status
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
	compliantClusters, nonCompliantClusters,
		unknownComplianceClusters := bundle.getChangedClusters(policy, originPolicyID)

	if len(compliantClusters)+len(nonCompliantClusters)+len(unknownComplianceClusters) == 0 {
		return nil, errPolicyStatusUnchanged // status flicker / CpP already covered it
	}

	return &statusbundle.PolicyGenericComplianceStatus{
		PolicyID:                  originPolicyID,
		CompliantClusters:         compliantClusters,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}, nil
}

// getChangedClusters returns arrays of changed compliance (cluster names).
func (bundle *DeltaComplianceStatusBundle) getChangedClusters(policy *v1.Policy,
	policyID string) ([]string, []string, []string) {
	compliantClusters := make([]string, 0)
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)

	for _, clusterCompliance := range policy.Status.Status {
		switch clusterCompliance.ComplianceState {
		case v1.Compliant:
			if bundle.complianceRecordsCache[policyID].compliantClustersSet.Contains(clusterCompliance.ClusterName) {
				continue
			}

			bundle.complianceRecordsCache[policyID].compliantClustersSet.Add(clusterCompliance.ClusterName)
			bundle.complianceRecordsCache[policyID].nonCompliantClustersSet.Remove(clusterCompliance.ClusterName)
			bundle.complianceRecordsCache[policyID].unknownClustersSet.Remove(clusterCompliance.ClusterName)

			compliantClusters = append(compliantClusters, clusterCompliance.ClusterName)
		case v1.NonCompliant:
			if bundle.complianceRecordsCache[policyID].nonCompliantClustersSet.Contains(clusterCompliance.ClusterName) {
				continue
			}

			bundle.complianceRecordsCache[policyID].compliantClustersSet.Remove(clusterCompliance.ClusterName)
			bundle.complianceRecordsCache[policyID].nonCompliantClustersSet.Add(clusterCompliance.ClusterName)
			bundle.complianceRecordsCache[policyID].unknownClustersSet.Remove(clusterCompliance.ClusterName)

			nonCompliantClusters = append(nonCompliantClusters, clusterCompliance.ClusterName)
		default:
			if bundle.complianceRecordsCache[policyID].unknownClustersSet.Contains(clusterCompliance.ClusterName) {
				continue
			}

			bundle.complianceRecordsCache[policyID].compliantClustersSet.Remove(clusterCompliance.ClusterName)
			bundle.complianceRecordsCache[policyID].nonCompliantClustersSet.Remove(clusterCompliance.ClusterName)
			bundle.complianceRecordsCache[policyID].unknownClustersSet.Add(clusterCompliance.ClusterName)

			unknownComplianceClusters = append(unknownComplianceClusters, clusterCompliance.ClusterName)
		}
	}

	return compliantClusters, nonCompliantClusters, unknownComplianceClusters
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
	bundle.complianceRecordsCache[status.PolicyID] = &policyComplianceStatus{
		compliantClustersSet:    createSetFromSlice(status.CompliantClusters),
		nonCompliantClustersSet: createSetFromSlice(status.NonCompliantClusters),
		unknownClustersSet:      createSetFromSlice(status.UnknownComplianceClusters),
	}
}
