package bundle

import (
	"sync"

	policiesv1 "github.com/open-cluster-management/governance-policy-propagator/api/v1"
	statusbundle "github.com/stolostron/hub-of-hubs-data-types/bundle/status"
)

// NewPolicyPlacementStatusBundle creates a new instance of PolicyPlacementStatusBundle.
func NewPolicyPlacementStatusBundle(leafHubName string, incarnation uint64,
	extractObjIDFunc ExtractObjIDFunc) Bundle {
	return &PolicyPlacementStatusBundle{
		BasePolicyPlacementStatusBundle: statusbundle.BasePolicyPlacementStatusBundle{
			Objects:       make([]*statusbundle.PolicyPlacementStatus, 0),
			LeafHubName:   leafHubName,
			BundleVersion: statusbundle.NewBundleVersion(incarnation, 0),
		},
		extractObjIDFunc: extractObjIDFunc,
		lock:             sync.Mutex{},
	}
}

// PolicyPlacementStatusBundle abstracts management of policy placement status bundle.
type PolicyPlacementStatusBundle struct {
	statusbundle.BasePolicyPlacementStatusBundle
	extractObjIDFunc ExtractObjIDFunc
	lock             sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *PolicyPlacementStatusBundle) UpdateObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	policy, isPolicy := object.(*policiesv1.Policy)
	if !isPolicy {
		return // do not handle objects other than policy
	}

	originPolicyID, ok := bundle.extractObjIDFunc(object)
	if !ok {
		return // cant update the object without finding its id.
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle
		bundle.Objects = append(bundle.Objects, bundle.getPolicyPlacement(originPolicyID, policy))
		bundle.BundleVersion.Generation++

		return
	}

	// if we reached here, object already exists in the bundle, check if the object has changed.
	if !bundle.updateObjectIfChanged(index, policy) {
		return // returns true if changed, otherwise false. if placement didn't change, don't increment generation.
	}

	// if policy placement has changed - update bundle generation
	bundle.BundleVersion.Generation++
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *PolicyPlacementStatusBundle) DeleteObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	_, isPolicy := object.(*policiesv1.Policy)
	if !isPolicy {
		return // do not handle objects other than policy
	}

	originPolicyID, ok := bundle.extractObjIDFunc(object)
	if !ok {
		return // cant update the object without finding its id.
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}

	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects
	bundle.BundleVersion.Generation++
}

// GetBundleVersion function to get bundle version.
func (bundle *PolicyPlacementStatusBundle) GetBundleVersion() *statusbundle.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.BundleVersion
}

func (bundle *PolicyPlacementStatusBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}

	return -1, errObjectNotFound
}

func (bundle *PolicyPlacementStatusBundle) getPolicyPlacement(originPolicyID string,
	policy *policiesv1.Policy) *statusbundle.PolicyPlacementStatus {
	return &statusbundle.PolicyPlacementStatus{
		PolicyID:        originPolicyID,
		Placement:       policy.Status.Placement,
		ResourceVersion: policy.GetResourceVersion(),
	}
}

// returns true if object was changed, otherwise returns false.
func (bundle *PolicyPlacementStatusBundle) updateObjectIfChanged(index int, policy *policiesv1.Policy) bool {
	if bundle.equals(bundle.Objects[index].Placement, policy.Status.Placement) {
		return false // if what's stored in the bundle equals to latest placement, do nothing.
	}

	// otherwise, update the policy placement object in the bundle
	bundle.Objects[index].Placement = policy.Status.Placement
	bundle.Objects[index].ResourceVersion = policy.GetResourceVersion()

	return true
}

func (bundle *PolicyPlacementStatusBundle) equals(placement1 []*policiesv1.Placement,
	placement2 []*policiesv1.Placement) bool {
	if len(placement1) != len(placement2) {
		return false
	}
	// length of the array equals. check for each item in placement array 1 if it exists in the placement array 2.
	for _, placementItem1 := range placement1 {
		if !bundle.contains(placement2, placementItem1) {
			return false
		}
	}

	return true
}

func (bundle *PolicyPlacementStatusBundle) contains(placement []*policiesv1.Placement,
	item *policiesv1.Placement) bool {
	for _, placementEntry := range placement {
		if item.Placement == placementEntry.Placement &&
			item.PlacementRule == placementEntry.PlacementRule &&
			item.PlacementBinding == placementEntry.PlacementBinding {
			return true
		}
	}

	return false
}
