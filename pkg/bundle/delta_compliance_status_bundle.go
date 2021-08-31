package bundle

import (
	"errors"
	"sync"

	v1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	statusbundle "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
)

const unknownComplianceStatus = "unknown"

var (
	errPolicyNotFromHubOfHubs = errors.New("policy wasn't sent from hub of hubs")
	errPolicyStatusUnchanged  = errors.New("policy status did not changed")
)

// NewDeltaComplianceStatusBundle creates a new instance of DeltaComplianceStatusBundle.
func NewDeltaComplianceStatusBundle(leafHubName string, generation uint64, clustersPerPolicyBaseBundle Bundle,
	completeComplianceBaseBundle HybridBundle, existingPoliciesMap map[string]struct{}) DeltaStateBundle {
	return &DeltaComplianceStatusBundle{
		BaseDeltaComplianceStatusBundle: statusbundle.BaseDeltaComplianceStatusBundle{
			Objects:                               make([]*statusbundle.PolicyDeltaComplianceStatus, 0),
			LeafHubName:                           leafHubName,
			ClustersPerPolicyBaseBundleGeneration: clustersPerPolicyBaseBundle.GetBundleGeneration(),
			ComplianceBaseBundleGeneration:        completeComplianceBaseBundle.GetBundleGeneration(),
			Generation:                            generation,
		},
		clustersPerPolicyBaseBundle: clustersPerPolicyBaseBundle,
		complianceBaseBundle:        completeComplianceBaseBundle,
		existingPoliciesMap:         existingPoliciesMap,
		policyComplianceRecords:     make(map[string]*policyComplianceOptimizedStatus),
		enabled:                     false,
		lock:                        sync.Mutex{},
	}
}

// DeltaComplianceStatusBundle abstracts management of compliance status bundle.
type DeltaComplianceStatusBundle struct {
	statusbundle.BaseDeltaComplianceStatusBundle
	clustersPerPolicyBaseBundle Bundle
	complianceBaseBundle        HybridBundle
	existingPoliciesMap         map[string]struct{}
	policyComplianceRecords     map[string]*policyComplianceOptimizedStatus
	enabled                     bool
	lock                        sync.Mutex
}

type policyComplianceOptimizedStatus struct {
	nonCompliantClustersMap map[string]struct{}
	unknownClustersMap      map[string]struct{}
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *DeltaComplianceStatusBundle) UpdateObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	if !bundle.enabled {
		return
	}

	policy, legal := object.(*v1.Policy)
	if !legal {
		return
	}

	originPolicyID, found := object.GetAnnotations()[datatypes.OriginOwnerReferenceAnnotation]
	if !found {
		return // origin owner reference annotation not found, not handling policy that wasn't sent from hub of hubs
	}

	policyComplianceObject, err := bundle.getPolicyComplianceStatus(originPolicyID, policy)
	if err != nil {
		return // error found means object should be skipped
	}

	// update only
	bundle.ClustersPerPolicyBaseBundleGeneration = bundle.clustersPerPolicyBaseBundle.GetBundleGeneration()

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle
		bundle.Objects = append(bundle.Objects, policyComplianceObject)
		bundle.Generation++

		return
	}

	// object found, update content
	bundle.updatePolicyComplianceStatus(index, policyComplianceObject)
	bundle.Generation++
}

// DeleteObject function to delete all instances of a single object inside a bundle.
func (bundle *DeltaComplianceStatusBundle) DeleteObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	if !bundle.enabled {
		return
	}

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
	delete(bundle.existingPoliciesMap, originPolicyID)
}

// GetBundleGeneration function to get bundle generation.
func (bundle *DeltaComplianceStatusBundle) GetBundleGeneration() uint64 {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.Generation
}

// GetObjectsCount function to get bundle objects count.
func (bundle *DeltaComplianceStatusBundle) GetObjectsCount() int {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return len(bundle.Objects)
}

// GetObjects function to return the hybrid bundle's objects.
func (bundle *DeltaComplianceStatusBundle) GetObjects() interface{} {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.Objects
}

// Enable function to sync bundle's recorded baseline generation and enable it for object updates.
func (bundle *DeltaComplianceStatusBundle) Enable() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.updateObjectRecordsFromBase()
	bundle.updateBaseBundlesGenerations()
	bundle.enabled = true
}

// Disable function to flush a bundle and prohibit it from taking object updates.
func (bundle *DeltaComplianceStatusBundle) Disable() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.enabled = false
	bundle.Objects = nil // safe after go1.0
}

// FlushOrderedObjects flushes a number of objects from the bundle.
func (bundle *DeltaComplianceStatusBundle) FlushOrderedObjects(count int) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	if count <= len(bundle.Objects) {
		bundle.Objects = bundle.Objects[count:]
	}
}

// updateObjectRecordsFromBase ensures that the delta state objects records are up-to-date when bundle gets enabled.
func (bundle *DeltaComplianceStatusBundle) updateObjectRecordsFromBase() {
	policiesInBaseBundleObjects := make(map[string]struct{})

	// create records from policies in base bundle's objects (has at least one non-compliant/unknown cluster).
	policiesCompleteComplianceStatus, ok :=
		bundle.complianceBaseBundle.GetObjects().([]*statusbundle.PolicyCompleteComplianceStatus)
	if !ok {
		return
	}

	for _, completeComplianceStatus := range policiesCompleteComplianceStatus {
		// check if policy record exists
		if _, policyRecordsExist :=
			bundle.policyComplianceRecords[completeComplianceStatus.PolicyID]; !policyRecordsExist {
			// no record exists
			bundle.policyComplianceRecords[completeComplianceStatus.PolicyID] = &policyComplianceOptimizedStatus{
				nonCompliantClustersMap: make(map[string]struct{}),
				unknownClustersMap:      make(map[string]struct{}),
			}
		}

		// update record content
		bundle.updateRecordedClusterComplianceStatus(&completeComplianceStatus.PolicyID, nil,
			completeComplianceStatus.NonCompliantClusters, completeComplianceStatus.UnknownComplianceClusters)

		policiesInBaseBundleObjects[completeComplianceStatus.PolicyID] = struct{}{}
	}

	// create records by inferring which policies are not in base bundle's objects but are compliant.
	for policyID := range bundle.existingPoliciesMap {
		if _, isInObjects := policiesInBaseBundleObjects[policyID]; isInObjects {
			continue
		}

		bundle.policyComplianceRecords[policyID] = &policyComplianceOptimizedStatus{
			nonCompliantClustersMap: make(map[string]struct{}),
			unknownClustersMap:      make(map[string]struct{}),
		}
	}
}

func (bundle *DeltaComplianceStatusBundle) updateBaseBundlesGenerations() {
	bundle.ClustersPerPolicyBaseBundleGeneration = bundle.clustersPerPolicyBaseBundle.GetBundleGeneration()
	bundle.ComplianceBaseBundleGeneration = bundle.complianceBaseBundle.GetBundleGeneration()
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
	policy *v1.Policy) (*statusbundle.PolicyDeltaComplianceStatus, error) {
	compliantClusters, nonCompliantClusters, unknownComplianceClusters, err := bundle.getChangedClusters(policy)
	if err != nil {
		return nil, err
	}

	if len(compliantClusters)+len(nonCompliantClusters)+len(unknownComplianceClusters) == 0 {
		return nil, errPolicyStatusUnchanged
	}

	return &statusbundle.PolicyDeltaComplianceStatus{
		PolicyID:                  originPolicyID,
		CompliantClusters:         compliantClusters,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
		ResourceVersion:           policy.GetResourceVersion(),
	}, nil
}

// updatePolicyComplianceStatus updates compliance statuses of an already listed policy object.
func (bundle *DeltaComplianceStatusBundle) updatePolicyComplianceStatus(policyIndex int,
	newPolicyStatus *statusbundle.PolicyDeltaComplianceStatus) {
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

// getChangedClusters returns arrays of changed compliance (cluster names).
func (bundle *DeltaComplianceStatusBundle) getChangedClusters(policy *v1.Policy) ([]string, []string, []string,
	error) {
	compliantClusters := make([]string, 0)
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)

	originPolicyID, found := policy.GetAnnotations()[datatypes.OriginOwnerReferenceAnnotation]
	if !found {
		// origin owner reference annotation not found, not handling policy that wasn't sent from hub of hubs
		return nil, nil, nil, errPolicyNotFromHubOfHubs
	}

	// if policy is new then all cluster statuses are assumed to previously have been unknown.
	if _, policyExistsInPoliciesMap := bundle.existingPoliciesMap[originPolicyID]; !policyExistsInPoliciesMap {
		// create new record
		bundle.existingPoliciesMap[originPolicyID] = struct{}{}
		bundle.createNewPolicyRecord(&originPolicyID, policy)
	}

	for _, clusterCompliance := range policy.Status.Status {
		if bundle.getRecordedClusterComplianceStatus(&originPolicyID, &clusterCompliance.ClusterName) ==
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

	return compliantClusters, nonCompliantClusters, unknownComplianceClusters, nil
}

// getRecordedClusterComplianceStatus gets the recorded compliance of a given cluster in a policy.
func (bundle *DeltaComplianceStatusBundle) getRecordedClusterComplianceStatus(originPolicyID *string,
	clusterName *string) v1.ComplianceState {
	policyRecords := bundle.policyComplianceRecords[*originPolicyID]

	if _, clusterExistsInNonCompliantMap :=
		policyRecords.nonCompliantClustersMap[*clusterName]; clusterExistsInNonCompliantMap {
		return v1.NonCompliant
	}

	if _, clusterExistsInUnknownMap :=
		policyRecords.unknownClustersMap[*clusterName]; clusterExistsInUnknownMap {
		return unknownComplianceStatus
	}

	return v1.Compliant // if not found in non-compliant/unknown then it is implicitly compliant
}

// updateRecordedClusterComplianceStatus updates the compliance records of a given policy.
func (bundle *DeltaComplianceStatusBundle) updateRecordedClusterComplianceStatus(originPolicyID *string,
	compliantClusters []string, nonCompliantClusters []string, unknownClusters []string) {
	policyRecords := bundle.policyComplianceRecords[*originPolicyID]

	for _, compliantCluster := range compliantClusters {
		delete(policyRecords.nonCompliantClustersMap, compliantCluster)
		delete(policyRecords.unknownClustersMap, compliantCluster)
	}

	for _, nonCompliantCluster := range nonCompliantClusters {
		policyRecords.nonCompliantClustersMap[nonCompliantCluster] = struct{}{}
		delete(policyRecords.unknownClustersMap, nonCompliantCluster)
	}

	for _, unknownCluster := range unknownClusters {
		policyRecords.unknownClustersMap[unknownCluster] = struct{}{}
		delete(policyRecords.nonCompliantClustersMap, unknownCluster)
	}
}

// createNewPolicyRecord creates a new record for the given policy and initiates all cluster states to unknown.
func (bundle *DeltaComplianceStatusBundle) createNewPolicyRecord(originPolicyID *string, policy *v1.Policy) {
	policyComplianceOptimizedStatus := &policyComplianceOptimizedStatus{
		nonCompliantClustersMap: make(map[string]struct{}),
		unknownClustersMap:      make(map[string]struct{}),
	}
	bundle.policyComplianceRecords[*originPolicyID] = policyComplianceOptimizedStatus

	for _, clusterCompliance := range policy.Status.Status {
		policyComplianceOptimizedStatus.unknownClustersMap[clusterCompliance.ClusterName] = struct{}{}
	}
}
