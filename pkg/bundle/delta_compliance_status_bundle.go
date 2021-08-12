package bundle

import (
	"errors"
	"sync"

	v1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	statusbundle "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
)

var errPolicyNotFromHubOfHubs = errors.New("policy wasn't sent from hub of hubs")

// NewDeltaComplianceStatusBundle creates a new instance of DeltaComplianceStatusBundle.
func NewDeltaComplianceStatusBundle(leafHubName string, generation uint64, baseBundle HybridBundle,
	policiesMap map[string]bool) HybridBundle {
	return &DeltaComplianceStatusBundle{
		BaseDeltaComplianceStatusBundle: statusbundle.BaseDeltaComplianceStatusBundle{
			Objects:              make([]*statusbundle.PolicyDeltaComplianceStatus, 0),
			LeafHubName:          leafHubName,
			BaseBundleGeneration: baseBundle.GetBundleGeneration(),
			Generation:           generation,
		},
		baseBundle:  baseBundle,
		policiesMap: policiesMap,
		enabled:     false,
		lock:        sync.Mutex{},
	}
}

// DeltaComplianceStatusBundle abstracts management of compliance status bundle.
type DeltaComplianceStatusBundle struct {
	statusbundle.BaseDeltaComplianceStatusBundle
	baseBundle  HybridBundle
	policiesMap map[string]bool
	enabled     bool
	lock        sync.Mutex
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

	bundle.Objects = append(bundle.Objects, policyComplianceObject)
	bundle.Generation++
}

// DeleteObject function to delete all instances of a single object inside a bundle.
func (bundle *DeltaComplianceStatusBundle) DeleteObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	if !bundle.enabled {
		return
	}

	// TODO: factor deletion into objects count (so that sent bundles are flushed correctly)
	bundle.deleteAllObjOccurrences(object)
}

// GetBundleGeneration function to get bundle generation.
func (bundle *DeltaComplianceStatusBundle) GetBundleGeneration() uint64 {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.Generation
}

// DeleteOrderedObjects function to delete a number of objects from the bundle.
func (bundle *DeltaComplianceStatusBundle) DeleteOrderedObjects(count int) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	if count <= len(bundle.Objects) {
		bundle.Objects = bundle.Objects[count:]
	}
}

// GetObjects function to return the hybrid bundle's objects.
func (bundle *DeltaComplianceStatusBundle) GetObjects() interface{} {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.Objects
}

// GetObjectsCount function to get bundle objects count.
func (bundle *DeltaComplianceStatusBundle) GetObjectsCount() int {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return len(bundle.Objects)
}

// Enable function to sync bundle's recorded baseline generation and enable it for object updates.
func (bundle *DeltaComplianceStatusBundle) Enable() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.BaseBundleGeneration = bundle.baseBundle.GetBundleGeneration()
	bundle.enabled = true
}

// Disable function to flush a bundle and prohibit it from taking object updates.
func (bundle *DeltaComplianceStatusBundle) Disable() {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.enabled = false
	bundle.Objects = nil // safe after go1.0
}

func (bundle *DeltaComplianceStatusBundle) deleteAllObjOccurrences(object Object) {
	originPolicyID, found := object.GetAnnotations()[datatypes.OriginOwnerReferenceAnnotation]
	if !found {
		return // origin owner reference annotation not found, cannot handle this policy
	}

	indexes, err := bundle.getObjectIndexesByUID(originPolicyID)
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}

	// do not increase generation, no need to send bundle when policy is removed (clusters per policy bundle is sent)
	for _, index := range indexes {
		bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects
	}
}

func (bundle *DeltaComplianceStatusBundle) getObjectIndexesByUID(uid string) ([]int, error) {
	indexes := make([]int, 0)

	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			indexes = append(indexes, i)
		}
	}

	if len(indexes) == 0 {
		return nil, errObjectNotFound
	}

	return indexes, nil
}

func (bundle *DeltaComplianceStatusBundle) getPolicyComplianceStatus(originPolicyID string,
	policy *v1.Policy) (*statusbundle.PolicyDeltaComplianceStatus, error) {
	compliantClusters, nonCompliantClusters, unknownComplianceClusters, err := bundle.getChangedClusters(policy)
	if err != nil {
		return nil, err
	}

	return &statusbundle.PolicyDeltaComplianceStatus{
		PolicyID:                  originPolicyID,
		CompliantClusters:         compliantClusters,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
		ResourceVersion:           policy.GetResourceVersion(),
	}, nil
}

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

	policyCompleteComplianceStatus, policyExistsInPoliciesMap := bundle.getBaselinePolicyObject(originPolicyID)

	for _, clusterCompliance := range policy.Status.Status {
		if bundle.getClusterComplianceStatusInBaseBundle(clusterCompliance.ClusterName,
			policyCompleteComplianceStatus, policyExistsInPoliciesMap) == clusterCompliance.ComplianceState {
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

	return compliantClusters, nonCompliantClusters, unknownComplianceClusters, nil
}

// getBaseLinePolicyStatus extracts the PolicyCompleteComplianceStatus from the baseline bundle.
// the returned tuple is (policyObject, policyExistsInPoliciesMap).
func (bundle *DeltaComplianceStatusBundle) getBaselinePolicyObject(originPolicyID string) (
	*statusbundle.PolicyCompleteComplianceStatus, bool) {
	for _, object := range bundle.baseBundle.GetObjects().([]*statusbundle.PolicyCompleteComplianceStatus) {
		if object.PolicyID == originPolicyID {
			return object, true
		}
	}

	// if we got here then the policy does not exist in the baseline.
	// this means it was either fully compliant or it is a new policy
	_, policyExistsInMap := bundle.policiesMap[originPolicyID]
	if policyExistsInMap {
		return nil, true
	}

	return nil, false
}

func (bundle *DeltaComplianceStatusBundle) getClusterComplianceStatusInBaseBundle(clusterName string,
	policyCompleteComplianceStatus *statusbundle.PolicyCompleteComplianceStatus,
	policyExistsInPoliciesMap bool) v1.ComplianceState {
	if policyCompleteComplianceStatus == nil {
		if policyExistsInPoliciesMap {
			return v1.Compliant
		}

		return "unknown"
	}

	for _, object := range policyCompleteComplianceStatus.NonCompliantClusters {
		if clusterName == object {
			return v1.NonCompliant
		}
	}

	for _, object := range policyCompleteComplianceStatus.UnknownComplianceClusters {
		if clusterName == object {
			return "unknown"
		}
	}

	return v1.Compliant // if not found in non-compliant/unknown then it is implicitly compliant
}
