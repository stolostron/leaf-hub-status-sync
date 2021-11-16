package bundle

import (
	"sync"

	policyv1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	statusbundle "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
)

// NewCompleteComplianceStatusBundle creates a new instance of ComplianceStatusBundle.
func NewCompleteComplianceStatusBundle(leafHubName string, baseBundle Bundle, generation uint64,
	extractObjIDFunc ExtractObjIDFunc) Bundle {
	return &ComplianceStatusBundle{
		BaseCompleteComplianceStatusBundle: statusbundle.BaseCompleteComplianceStatusBundle{
			Objects:              make([]*statusbundle.PolicyCompleteComplianceStatus, 0),
			LeafHubName:          leafHubName,
			BaseBundleGeneration: baseBundle.GetBundleGeneration(),
			Generation:           generation,
		},
		baseBundle:       baseBundle,
		lock:             sync.Mutex{},
		extractObjIDFunc: extractObjIDFunc,
	}
}

// ComplianceStatusBundle abstracts management of compliance status bundle.
type ComplianceStatusBundle struct {
	statusbundle.BaseCompleteComplianceStatusBundle
	baseBundle       Bundle
	lock             sync.Mutex
	extractObjIDFunc ExtractObjIDFunc
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *ComplianceStatusBundle) UpdateObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	// if cluster per policy is sent, no need to send complete compliance as well.
	// therefore we check if base generation was changed and decide whether we need to increase generation or not.
	previousBaseBundleGeneration := bundle.BaseBundleGeneration // used to check if clusters per policy (base) changed.
	bundle.BaseBundleGeneration = bundle.baseBundle.GetBundleGeneration()

	policy, ok := object.(*policyv1.Policy)
	if !ok {
		return // do not handle objects other than policy
	}

	originPolicyID, ok := bundle.extractObjIDFunc(object)
	if !ok {
		return // cant update the object without finding its id.
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle
		policyComplianceObject := bundle.getPolicyComplianceStatus(originPolicyID, policy)
		// don't send in the bundle a policy where all clusters are compliant
		if bundle.containsNonCompliantOrUnknownClusters(policyComplianceObject) {
			bundle.Objects = append(bundle.Objects, policyComplianceObject)
		}

		if previousBaseBundleGeneration == bundle.BaseBundleGeneration {
			bundle.Generation++ // clusters per policy doesn't increase generation on update(only in add/remove policy)
		} // increase generation only if base generation didn't increase during the update of this object

		return
	}
	// if we reached here, policy already exists in the bundle with at least one non compliant or unknown cluster.
	if !bundle.updateBundleIfObjectChanged(index, policy) {
		return // true if changed, otherwise false. if policy compliance didn't change don't increment generation.
	}

	// don't send in the bundle a policy where all clusters are compliant
	if !bundle.containsNonCompliantOrUnknownClusters(bundle.Objects[index]) {
		bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects
	}
	// increase bundle generation in the case where cluster lists were changed and base generation didn't increase
	if previousBaseBundleGeneration == bundle.BaseBundleGeneration {
		bundle.Generation++ // clusters per policy doesn't increase generation on update (only in add/remove policy).
	}
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *ComplianceStatusBundle) DeleteObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.BaseBundleGeneration = bundle.baseBundle.GetBundleGeneration()

	_, ok := object.(*policyv1.Policy)
	if !ok {
		return // do not handle objects other than policy
	}

	originPolicyID, ok := bundle.extractObjIDFunc(object)
	if !ok {
		return // cant delete the object without its id.
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}

	// do not increase generation, no need to send bundle when policy is removed (clusters per policy bundle is sent).
	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects
}

// GetBundleGeneration function to get bundle generation.
func (bundle *ComplianceStatusBundle) GetBundleGeneration() uint64 {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.Generation
}

func (bundle *ComplianceStatusBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}

	return -1, errObjectNotFound
}

func (bundle *ComplianceStatusBundle) getPolicyComplianceStatus(originPolicyID string,
	policy *policyv1.Policy) *statusbundle.PolicyCompleteComplianceStatus {
	nonCompliantClusters, unknownComplianceClusters := bundle.getNonCompliantAndUnknownClusters(policy)

	return &statusbundle.PolicyCompleteComplianceStatus{
		PolicyID:                  originPolicyID,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}
}

// returns a list of non compliant clusters and a list of unknown compliance clusters.
func (bundle *ComplianceStatusBundle) getNonCompliantAndUnknownClusters(policy *policyv1.Policy) ([]string, []string) {
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)

	for _, clusterCompliance := range policy.Status.Status {
		if clusterCompliance.ComplianceState == policyv1.Compliant {
			continue
		}

		if clusterCompliance.ComplianceState == policyv1.NonCompliant {
			nonCompliantClusters = append(nonCompliantClusters, clusterCompliance.ClusterName)
		} else { // not compliant not non compliant -> means unknown
			unknownComplianceClusters = append(unknownComplianceClusters, clusterCompliance.ClusterName)
		}
	}

	return nonCompliantClusters, unknownComplianceClusters
}

// if a cluster was removed, object is not considered as changed.
func (bundle *ComplianceStatusBundle) updateBundleIfObjectChanged(objectIndex int, policy *policyv1.Policy) bool {
	oldPolicyComplianceStatus := bundle.Objects[objectIndex]
	newNonCompliantClusters, newUnknownComplianceClusters := bundle.getNonCompliantAndUnknownClusters(policy)

	if !bundle.clusterListsEqual(oldPolicyComplianceStatus.NonCompliantClusters, newNonCompliantClusters) ||
		!bundle.clusterListsEqual(oldPolicyComplianceStatus.UnknownComplianceClusters, newUnknownComplianceClusters) {
		oldPolicyComplianceStatus.NonCompliantClusters = newNonCompliantClusters
		oldPolicyComplianceStatus.UnknownComplianceClusters = newUnknownComplianceClusters

		return true
	}

	return false
}

func (bundle *ComplianceStatusBundle) clusterListsEqual(oldClusters []string, newClusters []string) bool {
	if len(oldClusters) != len(newClusters) {
		return false
	}

	for _, newClusterName := range newClusters {
		if !helpers.ContainsString(oldClusters, newClusterName) {
			return false
		}
	}

	return true
}

func (bundle *ComplianceStatusBundle) containsNonCompliantOrUnknownClusters(
	policyComplianceStatus *statusbundle.PolicyCompleteComplianceStatus) bool {
	if len(policyComplianceStatus.UnknownComplianceClusters) == 0 &&
		len(policyComplianceStatus.NonCompliantClusters) == 0 {
		return false
	}

	return true
}
