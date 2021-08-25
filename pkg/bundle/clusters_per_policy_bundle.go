package bundle

import (
	"sync"

	policiesv1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	statusbundle "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/pkg/errors"
)

// NewClustersPerPolicyBundle creates a new instance of ClustersPerPolicyBundle.
func NewClustersPerPolicyBundle(leafHubName string, generation uint64) Bundle {
	return &ClustersPerPolicyBundle{
		BaseClustersPerPolicyBundle: statusbundle.BaseClustersPerPolicyBundle{
			Objects:     make([]*statusbundle.ClustersPerPolicy, 0),
			LeafHubName: leafHubName,
			Generation:  generation,
		},
		lock: sync.Mutex{},
	}
}

// ClustersPerPolicyBundle abstracts management of clusters per policy bundle.
type ClustersPerPolicyBundle struct {
	statusbundle.BaseClustersPerPolicyBundle
	lock sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *ClustersPerPolicyBundle) UpdateObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	policy, ok := object.(*policiesv1.Policy)
	if !ok {
		return // do not handle objects other than policy
	}

	// originPolicyID, found := object.GetAnnotations()[datatypes.OriginOwnerReferenceAnnotation]
	// if !found {
	//	return // origin owner reference annotation not found, not handling this policy (wasn't sent from hub of hubs)
	// }

	originPolicyID := string(policy.UID)

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle
		bundle.Objects = append(bundle.Objects, bundle.getClustersPerPolicy(originPolicyID, policy))
		bundle.Generation++

		return
	}

	// if we reached here, object already exists in the bundle, check if the object has changed.
	if object.GetResourceVersion() <= bundle.Objects[index].ResourceVersion {
		return // update object only if there is a newer version. check for changes using resourceVersion field
	}

	if !bundle.updateObjectIfChanged(index, bundle.getClusterNames(policy), policy.Spec.RemediationAction) {
		return // returns true if changed, otherwise false. if cluster list didn't change, don't increment generation.
	}
	// if cluster list has changed - update resource version of the object and bundle generation
	bundle.Objects[index].ResourceVersion = object.GetResourceVersion()
	bundle.Generation++
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *ClustersPerPolicyBundle) DeleteObject(object Object) {
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

	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects
	bundle.Generation++
}

// GetBundleGeneration function to get bundle generation.
func (bundle *ClustersPerPolicyBundle) GetBundleGeneration() uint64 {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.Generation
}

func (bundle *ClustersPerPolicyBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}

	return -1, errors.New("object not found")
}

func (bundle *ClustersPerPolicyBundle) getClusterNames(policy *policiesv1.Policy) []string {
	clusterNames := make([]string, len(policy.Status.Status))
	for i, clusterStatus := range policy.Status.Status {
		clusterNames[i] = clusterStatus.ClusterName
	}

	return clusterNames
}

func (bundle *ClustersPerPolicyBundle) getClustersPerPolicy(originPolicyID string,
	policy *policiesv1.Policy) *statusbundle.ClustersPerPolicy {
	return &statusbundle.ClustersPerPolicy{
		PolicyID:          originPolicyID,
		Clusters:          bundle.getClusterNames(policy),
		RemediationAction: policy.Spec.RemediationAction,
		ResourceVersion:   policy.GetResourceVersion(),
	}
}

func (bundle *ClustersPerPolicyBundle) updateObjectIfChanged(objectIndex int, newClusterNames []string,
	remediationAction policiesv1.RemediationAction) bool {
	oldClusterNames := bundle.Objects[objectIndex].Clusters
	for _, newClusterName := range newClusterNames {
		if !helpers.ContainsString(oldClusterNames, newClusterName) {
			bundle.Objects[objectIndex].Clusters = newClusterNames // we found a new cluster, update and mark as changed
			bundle.Objects[objectIndex].RemediationAction = remediationAction

			return true // if we update clusters, update remediation as well without checking
		}
	}
	// if we finished for loop, all new clusters can be found inside the existing clusters per policy list.
	// need to make sure there are no other clusters which are not relevant anymore (removed ones).
	// comparing length, if not equal there is at least one old cluster which is not relevant anymore.
	if len(oldClusterNames) != len(newClusterNames) {
		bundle.Objects[objectIndex].Clusters = newClusterNames
		bundle.Objects[objectIndex].RemediationAction = remediationAction

		return true // if we update clusters, update remediation as well without checking
	}
	// check if remediation action was changed or not
	if bundle.Objects[objectIndex].RemediationAction != remediationAction {
		bundle.Objects[objectIndex].RemediationAction = remediationAction // no need to update clusters, identical
		return true
	}

	return false
}
