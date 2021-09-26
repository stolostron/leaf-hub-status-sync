package bundle

import (
	"sync"

	v1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	statusbundle "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
)

// NewCompleteComplianceStatusBundle creates a new instance of CompleteComplianceStatusBundle.
func NewCompleteComplianceStatusBundle(leafHubName string, incarnation uint64, generation uint64, baseBundle Bundle,
	existingPoliciesMap map[string]struct{}) HybridBundle {
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
	existingPoliciesMap map[string]struct{}
	enabled             bool
	lock                sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *CompleteComplianceStatusBundle) UpdateObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	if !bundle.enabled {
		return
	}

	bundle.BaseBundleGeneration = bundle.baseBundle.GetBundleGeneration()

	policy, legal := object.(*v1.Policy)
	if !legal {
		return
	}

	originPolicyID, found := object.GetAnnotations()[datatypes.OriginOwnerReferenceAnnotation]
	if !found {
		return // origin owner reference annotation not found, not handling policy that wasn't sent from hub of hubs
	}

	// add to policies map so delta bundles are able to infer statuses correctly
	if _, exists := bundle.existingPoliciesMap[originPolicyID]; !exists {
		bundle.existingPoliciesMap[originPolicyID] = struct{}{}
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle
		policyComplianceObject := bundle.getPolicyComplianceStatus(originPolicyID, policy)
		// don't send in the bundle a policy where all clusters are compliant
		if bundle.containsNonCompliantOrUnknownClusters(policyComplianceObject) {
			bundle.Objects = append(bundle.Objects, policyComplianceObject)
		}
		bundle.Generation++

		return
	}

	// if we reached here, object already exists in the bundle with at least one non-compliant or unknown cluster.
	if object.GetResourceVersion() <= bundle.Objects[index].ResourceVersion { // check if the object has changed.
		return // update object only if there is a newer version. check for changes using resourceVersion field
	}

	if !bundle.updateBundleIfObjectChanged(index, policy) {
		return // true if changed, otherwise false. if policy compliance didn't change don't increment generation.
	}

	// don't send in the bundle a policy where all clusters are compliant
	if !bundle.containsNonCompliantOrUnknownClusters(bundle.Objects[index]) {
		bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects
	} else { // we have at least one cluster non compliant or unknown and cluster list has changed
		bundle.Objects[index].ResourceVersion = object.GetResourceVersion() // update resource version of the object
	}

	// increase bundle generation in the case where cluster lists were changed
	bundle.Generation++
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

	// do not increase generation, no need to send bundle when policy is removed (clusters per policy bundle is sent)
	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects

	delete(bundle.existingPoliciesMap, originPolicyID)
}

// GetBundleGeneration function to get bundle's generation.
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
	policy *v1.Policy) *statusbundle.PolicyCompleteComplianceStatus {
	nonCompliantClusters, unknownComplianceClusters := bundle.getNonCompliantAndUnknownClusters(policy)

	return &statusbundle.PolicyCompleteComplianceStatus{
		PolicyID:                  originPolicyID,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
		ResourceVersion:           policy.GetResourceVersion(),
	}
}

// returns a list of non compliant clusters and a list of unknown compliance clusters.
func (bundle *CompleteComplianceStatusBundle) getNonCompliantAndUnknownClusters(policy *v1.Policy) ([]string,
	[]string) {
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)

	for _, clusterCompliance := range policy.Status.Status {
		if clusterCompliance.ComplianceState == v1.Compliant {
			continue
		}

		if clusterCompliance.ComplianceState == v1.NonCompliant {
			nonCompliantClusters = append(nonCompliantClusters, clusterCompliance.ClusterName)
		} else { // not compliant not non compliant -> means unknown
			unknownComplianceClusters = append(unknownComplianceClusters, clusterCompliance.ClusterName)
		}
	}

	return nonCompliantClusters, unknownComplianceClusters
}

// if a cluster was removed, object is not considered as changed.
func (bundle *CompleteComplianceStatusBundle) updateBundleIfObjectChanged(objectIndex int, policy *v1.Policy) bool {
	oldPolicyComplianceStatus := bundle.Objects[objectIndex]
	newNonCompliantClusters, newUnknownComplianceClusters := bundle.getNonCompliantAndUnknownClusters(policy)
	// comparing length, if not equal there is at least one cluster that it's compliance status was changed.
	if len(oldPolicyComplianceStatus.NonCompliantClusters) != len(newNonCompliantClusters) ||
		len(oldPolicyComplianceStatus.UnknownComplianceClusters) != len(newUnknownComplianceClusters) {
		bundle.Objects[objectIndex].NonCompliantClusters = newNonCompliantClusters
		bundle.Objects[objectIndex].UnknownComplianceClusters = newUnknownComplianceClusters

		return true
	}
	// otherwise the length of the lists are equals (old and new lists per type).
	// check each cluster in the new list (could be that new cluster isn't in the list and old one was added and
	// therefore length is equal
	for _, nonCompliantCluster := range newNonCompliantClusters {
		if !helpers.ContainsString(oldPolicyComplianceStatus.NonCompliantClusters, nonCompliantCluster) {
			bundle.Objects[objectIndex].NonCompliantClusters = newNonCompliantClusters
			bundle.Objects[objectIndex].UnknownComplianceClusters = newUnknownComplianceClusters

			return true
		}
	}

	for _, unknownComplianceCluster := range newUnknownComplianceClusters {
		if !helpers.ContainsString(oldPolicyComplianceStatus.UnknownComplianceClusters, unknownComplianceCluster) {
			bundle.Objects[objectIndex].NonCompliantClusters = newNonCompliantClusters
			bundle.Objects[objectIndex].UnknownComplianceClusters = newUnknownComplianceClusters

			return true
		}
	}
	// if we finished the two for loops, all non compliant and unknown can be found inside the existing lists.
	return false
}

func (bundle *CompleteComplianceStatusBundle) containsNonCompliantOrUnknownClusters(
	policyComplianceStatus *statusbundle.PolicyCompleteComplianceStatus) bool {
	if len(policyComplianceStatus.UnknownComplianceClusters) == 0 &&
		len(policyComplianceStatus.NonCompliantClusters) == 0 {
		return false
	}

	return true
}
