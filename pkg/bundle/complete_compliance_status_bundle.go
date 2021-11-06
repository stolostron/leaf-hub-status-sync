package bundle

import (
	"sync"

	policyv1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	statusbundle "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
)

// NewCompleteComplianceStatusBundle creates a new instance of CompleteComplianceStatusBundle.
func NewCompleteComplianceStatusBundle(leafHubName string, incarnation uint64, generation uint64, baseBundle Bundle,
	existingPoliciesMap map[string]map[string]struct{}) HybridBundle {
	return &CompleteComplianceStatusBundle{
		BaseCompleteComplianceStatusBundle: statusbundle.BaseCompleteComplianceStatusBundle{
			Objects:              make([]*statusbundle.PolicyCompleteComplianceStatus, 0),
			LeafHubName:          leafHubName,
			BaseBundleGeneration: baseBundle.GetBundleGeneration(),
			BundleVersion:        *statusbundle.NewBundleVersion(incarnation, generation),
		},
		baseBundle:          baseBundle,
		existingPoliciesMap: existingPoliciesMap,
		enabled:             false,
		lock:                sync.Mutex{},
	}
}

// CompleteComplianceStatusBundle abstracts management of compliance status bundle.
type CompleteComplianceStatusBundle struct {
	statusbundle.BaseCompleteComplianceStatusBundle
	baseBundle          Bundle
	existingPoliciesMap map[string]map[string]struct{}
	enabled             bool
	lock                sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *CompleteComplianceStatusBundle) UpdateObject(object Object) {
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

	originPolicyID, found := object.GetAnnotations()[datatypes.OriginOwnerReferenceAnnotation]
	if !found {
		return // origin owner reference annotation not found, not handling policy that wasn't sent from hub of hubs
	}

	// add to policies map so delta bundles are able to infer statuses correctly
	if _, exists := bundle.existingPoliciesMap[originPolicyID]; !exists {
		bundle.existingPoliciesMap[originPolicyID] = make(map[string]struct{})
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
func (bundle *CompleteComplianceStatusBundle) DeleteObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	if !bundle.enabled {
		return
	}

	bundle.BaseBundleGeneration = bundle.baseBundle.GetBundleGeneration()

	originPolicyID, found := object.GetAnnotations()[datatypes.OriginOwnerReferenceAnnotation]
	if !found {
		return // origin owner reference annotation not found, cannot handle this policy
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}

	// do not increase generation, no need to send bundle when policy is removed (clusters per policy bundle is sent).
	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects

	delete(bundle.existingPoliciesMap, originPolicyID)
}

// GetBundleGeneration function to get bundle generation.
func (bundle *CompleteComplianceStatusBundle) GetBundleGeneration() uint64 {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.Generation
}

// GetObjectsCount function to get bundle objects count.
func (bundle *CompleteComplianceStatusBundle) GetObjectsCount() int {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return len(bundle.Objects)
}

// GetObjects returns the bundle's objects.
func (bundle *CompleteComplianceStatusBundle) GetObjects() interface{} {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.Objects
}

// Enable function to sync bundle's recorded baseline generation and enable it for object updates.
func (bundle *CompleteComplianceStatusBundle) Enable() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.BaseBundleGeneration = bundle.baseBundle.GetBundleGeneration()
	bundle.enabled = true
}

// Disable function to flush a bundle and prohibit it from taking object updates.
func (bundle *CompleteComplianceStatusBundle) Disable() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.enabled = false
	bundle.Objects = nil // safe after go1.0
}

func (bundle *CompleteComplianceStatusBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}

	return -1, errObjectNotFound
}

func (bundle *CompleteComplianceStatusBundle) getPolicyComplianceStatus(originPolicyID string,
	policy *policyv1.Policy) *statusbundle.PolicyCompleteComplianceStatus {
	nonCompliantClusters, unknownComplianceClusters := bundle.getNonCompliantAndUnknownClusters(policy)

	return &statusbundle.PolicyCompleteComplianceStatus{
		PolicyID:                  originPolicyID,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}
}

// returns a list of non compliant clusters and a list of unknown compliance clusters.
func (bundle *CompleteComplianceStatusBundle) getNonCompliantAndUnknownClusters(policy *policyv1.Policy) ([]string, []string) {
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
func (bundle *CompleteComplianceStatusBundle) updateBundleIfObjectChanged(objectIndex int, policy *policyv1.Policy) bool {
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

func (bundle *CompleteComplianceStatusBundle) clusterListsEqual(oldClusters []string, newClusters []string) bool {
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

func (bundle *CompleteComplianceStatusBundle) containsNonCompliantOrUnknownClusters(
	policyComplianceStatus *statusbundle.PolicyCompleteComplianceStatus) bool {
	if len(policyComplianceStatus.UnknownComplianceClusters) == 0 &&
		len(policyComplianceStatus.NonCompliantClusters) == 0 {
		return false
	}

	return true
}
