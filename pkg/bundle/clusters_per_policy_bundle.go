package bundle

import (
	policiesv1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sync"
)

func NewClustersPerPolicyBundle(leafHubName string) Bundle {
	return &ClustersPerPolicyBundle{
		Objects:     make([]*ClustersPerPolicy, 0),
		LeafHubName: leafHubName,
		generation:  0,
		lock:        sync.Mutex{},
	}
}

type ClustersPerPolicy struct {
	PolicyId        types.UID `json:"policyId"`
	Clusters        []string  `json:"clusters"`
	ResourceVersion string    `json:"resourceVersion"`
}

type ClustersPerPolicyBundle struct {
	Objects     []*ClustersPerPolicy `json:"objects"`
	LeafHubName string               `json:"leafHubName"`
	generation  uint64
	lock        sync.Mutex
}

func (bundle *ClustersPerPolicyBundle) UpdateObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	policy := object.(*policiesv1.Policy)
	index, err := bundle.getObjectIndexByUID(object.GetUID())
	if err != nil { // object not found, need to add it to the bundle
		bundle.Objects = append(bundle.Objects, bundle.getClustersPerPolicy(policy))
		bundle.generation++
		return
	}

	// if we reached here, object already exists in the bundle.. check if the object has changed.
	if object.GetResourceVersion() <= bundle.Objects[index].ResourceVersion {
		return // update object only if there is a newer version. check for changes using resourceVersion field
	}

	if !bundle.updateClusterListIfChanged(index, bundle.getClusterNames(policy)) {
		return //returns true if changed, otherwise false. if cluster list didn't change, don't increment generation.
	}
	// if cluster list has changed - update resource version of the object and bundle generation
	bundle.Objects[index].ResourceVersion = object.GetResourceVersion()
	bundle.generation++
}

func (bundle *ClustersPerPolicyBundle) DeleteObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	index, err := bundle.getObjectIndexByUID(object.GetUID())
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}
	bundle.Objects = append(bundle.Objects[:index], bundle.Objects[index+1:]...) // remove from objects
	bundle.generation++
}

func (bundle *ClustersPerPolicyBundle) GetBundleGeneration() uint64 {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.generation
}
func (bundle *ClustersPerPolicyBundle) getObjectIndexByUID(uid types.UID) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyId == uid {
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

func (bundle *ClustersPerPolicyBundle) getClustersPerPolicy(policy *policiesv1.Policy) *ClustersPerPolicy {
	return &ClustersPerPolicy{
		PolicyId:        policy.GetUID(),
		Clusters:        bundle.getClusterNames(policy),
		ResourceVersion: policy.GetResourceVersion(),
	}
}

func (bundle *ClustersPerPolicyBundle) updateClusterListIfChanged(objectIndex int, newClusterNames []string) bool {
	oldClusterNames := bundle.Objects[objectIndex].Clusters
	for _, newClusterName := range newClusterNames {
		if !helpers.ContainsString(oldClusterNames, newClusterName) {
			bundle.Objects[objectIndex].Clusters = newClusterNames // we found a new cluster, update and mark as changed
			return true
		}
	}
	// if we finished for loop, all new clusters can be found inside the existing clusters per policy list.
	// need to make sure there are no other clusters which are not relevant anymore (removed ones).
	// comparing length, if not equal there is at least one old cluster which is not relevant anymore.
	if len(oldClusterNames) != len(newClusterNames) {
		bundle.Objects[objectIndex].Clusters = newClusterNames
		return true
	}
	return false
}
