package bundle

import (
	"sync"

	policiesv1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	statusbundle "github.com/open-cluster-management/hub-of-hubs-data-types/bundle/status"
	"github.com/pkg/errors"
)

// NewClustersPerPolicyBundle creates a new instance of ClustersPerPolicyBundle.
func NewClustersPerPolicyBundle(leafHubName string, generation uint64, extractObjIDFunc ExtractObjIDFunc) Bundle {
	return &ClustersPerPolicyBundle{
		BaseClustersPerPolicyBundle: statusbundle.BaseClustersPerPolicyBundle{
			Objects:     make([]*statusbundle.PolicyGenericComplianceStatus, 0),
			LeafHubName: leafHubName,
			Generation:  generation,
		},
		extractObjIDFunc: extractObjIDFunc,
		lock:             sync.Mutex{},
	}
}

// ClustersPerPolicyBundle abstracts management of clusters per policy bundle.
type ClustersPerPolicyBundle struct {
	statusbundle.BaseClustersPerPolicyBundle
	extractObjIDFunc ExtractObjIDFunc
	lock             sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *ClustersPerPolicyBundle) UpdateObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	policy, ok := object.(*policiesv1.Policy)
	if !ok {
		return // do not handle objects other than policy
	}

	originPolicyID, ok := bundle.extractObjIDFunc(object)
	if !ok {
		return // cant update the object without finding its id.
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle
		bundle.Objects = append(bundle.Objects, bundle.getClustersPerPolicy(originPolicyID, policy))
		bundle.Generation++

		return
	}
	// when we update a policy, we need to increase bundle generation only if cluster list of the policy has changed.
	// for the use case where no cluster was added/removed, we use the status compliance bundle to update hub of hubs
	// and not the clusters per policy bundle which contains a lot more information (full state).
	//
	// that being said, we still want to update the internal data and keep it always up to date in case a policy will be
	// inserted/removed (or cluster added/removed) and full state bundle will be triggered.
	if bundle.updateObjectIfChanged(index, policy) { // returns true if cluster list has changed, otherwise false
		bundle.Generation++
	}
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *ClustersPerPolicyBundle) DeleteObject(object Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	_, ok := object.(*policiesv1.Policy)
	if !ok {
		return // wont handle anything other than policies
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

// getClusterStatuses returns (list of compliant clusters, list of nonCompliant clusters, list of unknown clusters,
// list of all clusters).
func (bundle *ClustersPerPolicyBundle) getClusterStatuses(policy *policiesv1.Policy) ([]string, []string, []string,
	[]string) {
	compliantClusters := make([]string, 0)
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)
	allClusters := make([]string, 0)

	for _, clusterStatus := range policy.Status.Status {
		if clusterStatus.ComplianceState == policiesv1.Compliant {
			compliantClusters = append(compliantClusters, clusterStatus.ClusterName)
			allClusters = append(allClusters, clusterStatus.ClusterName)

			continue
		}
		// else
		if clusterStatus.ComplianceState == policiesv1.NonCompliant {
			nonCompliantClusters = append(nonCompliantClusters, clusterStatus.ClusterName)
			allClusters = append(allClusters, clusterStatus.ClusterName)

			continue
		}
		// else
		unknownComplianceClusters = append(unknownComplianceClusters, clusterStatus.ClusterName)
		allClusters = append(allClusters, clusterStatus.ClusterName)
	}

	return compliantClusters, nonCompliantClusters, unknownComplianceClusters, allClusters
}

func (bundle *ClustersPerPolicyBundle) getClustersPerPolicy(originPolicyID string,
	policy *policiesv1.Policy) *statusbundle.PolicyGenericComplianceStatus {
	compliantClusters, nonCompliantClusters, unknownComplianceClusters, _ := bundle.getClusterStatuses(policy)

	return &statusbundle.PolicyGenericComplianceStatus{
		PolicyID:                  originPolicyID,
		CompliantClusters:         compliantClusters,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}
}

// returns true if cluster list has changed, otherwise returns false (even if cluster statuses changed).
func (bundle *ClustersPerPolicyBundle) updateObjectIfChanged(objectIndex int, policy *policiesv1.Policy) bool {
	newCompliantClusters, newNonCompliantClusters, newUnknownClusters, newClusters := bundle.getClusterStatuses(policy)
	oldPolicyStatus := bundle.Objects[objectIndex]
	clusterListChanged := false

	// check if any cluster was added or removed
	if len(oldPolicyStatus.CompliantClusters)+len(oldPolicyStatus.NonCompliantClusters)+
		len(oldPolicyStatus.UnknownComplianceClusters) != len(newClusters) {
		clusterListChanged = true // if the length is different, at least one cluster was added/removed
	} else if !bundle.clusterListContains(oldPolicyStatus.NonCompliantClusters, newClusters) ||
		!bundle.clusterListContains(oldPolicyStatus.CompliantClusters, newClusters) ||
		!bundle.clusterListContains(oldPolicyStatus.UnknownComplianceClusters, newClusters) {
		clusterListChanged = true // at least one cluster was added/removed
	}

	// in any case we want to update the internal bundle in case statuses changed
	oldPolicyStatus.CompliantClusters = newCompliantClusters
	oldPolicyStatus.NonCompliantClusters = newNonCompliantClusters
	oldPolicyStatus.UnknownComplianceClusters = newUnknownClusters

	return clusterListChanged
}

func (bundle *ClustersPerPolicyBundle) clusterListContains(subsetClusters []string, allClusters []string) bool {
	for _, clusterName := range subsetClusters {
		if !ContainsString(allClusters, clusterName) {
			return false
		}
	}

	return true
}
