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
func NewClustersPerPolicyBundle(leafHubName string, incarnation uint64, generation uint64) Bundle {
	return &ClustersPerPolicyBundle{
		BaseClustersPerPolicyBundle: statusbundle.BaseClustersPerPolicyBundle{
			Objects:       make([]*statusbundle.PolicyGenericComplianceStatus, 0),
			LeafHubName:   leafHubName,
			BundleVersion: *statusbundle.NewBundleVersion(incarnation, generation),
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

	originPolicyID, found := object.GetAnnotations()[datatypes.OriginOwnerReferenceAnnotation]
	if !found {
		return // origin owner reference annotation not found, not handling this policy (wasn't sent from hub of hubs)
	}

	index, err := bundle.getObjectIndexByUID(originPolicyID)
	if err != nil { // object not found, need to add it to the bundle
		bundle.Objects = append(bundle.Objects, bundle.getClustersPerPolicy(originPolicyID, policy))
		bundle.Generation++

		return
	}
	// when we update object (if changed), no need to increase generation.
	// for this use case where no policy was added/removed, we use the status compliance bundle to update hub of hubs
	// and not the clusters per policy bundle which contains a lot more information (full state).
	//
	// that being said, we still want to update the internal data and keep it always up to date in case a policy will be
	// inserted/removed and full state bundle will be triggered.
	bundle.updateObjectIfChanged(index, policy)
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

// GetObjectsCount function to get bundle objects count.
func (bundle *ClustersPerPolicyBundle) GetObjectsCount() int {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return len(bundle.Objects)
}

func (bundle *ClustersPerPolicyBundle) getObjectIndexByUID(uid string) (int, error) {
	for i, object := range bundle.Objects {
		if object.PolicyID == uid {
			return i, nil
		}
	}

	return -1, errors.New("object not found")
}

// getClusterStatuses returns (list of compliant clusters, list of nonCompliant clusters, list of unknown clusters).
func (bundle *ClustersPerPolicyBundle) getClusterStatuses(policy *policiesv1.Policy) ([]string, []string, []string) {
	compliantClusters := make([]string, 0)
	nonCompliantClusters := make([]string, 0)
	unknownComplianceClusters := make([]string, 0)

	for _, clusterStatus := range policy.Status.Status {
		if clusterStatus.ComplianceState == policiesv1.Compliant {
			compliantClusters = append(compliantClusters, clusterStatus.ClusterName)
			continue
		} // else

		if clusterStatus.ComplianceState == policiesv1.NonCompliant {
			nonCompliantClusters = append(nonCompliantClusters, clusterStatus.ClusterName)
			continue
		} // else

		unknownComplianceClusters = append(unknownComplianceClusters, clusterStatus.ClusterName)
	}

	return compliantClusters, nonCompliantClusters, unknownComplianceClusters
}

func (bundle *ClustersPerPolicyBundle) getClustersPerPolicy(originPolicyID string,
	policy *policiesv1.Policy) *statusbundle.PolicyGenericComplianceStatus {
	compliantClusters, nonCompliantClusters, unknownComplianceClusters := bundle.getClusterStatuses(policy)

	return &statusbundle.PolicyGenericComplianceStatus{
		PolicyID:                  originPolicyID,
		CompliantClusters:         compliantClusters,
		NonCompliantClusters:      nonCompliantClusters,
		UnknownComplianceClusters: unknownComplianceClusters,
	}
}

func (bundle *ClustersPerPolicyBundle) updateObjectIfChanged(objectIndex int, policy *policiesv1.Policy) {
	newCompliantClusters, newNonCompliantClusters, newUnknownComplianceClusters := bundle.getClusterStatuses(policy)
	oldPolicyStatus := bundle.Objects[objectIndex]

	if !bundle.clusterListsEqual(oldPolicyStatus.CompliantClusters, newCompliantClusters) ||
		!bundle.clusterListsEqual(oldPolicyStatus.NonCompliantClusters, newNonCompliantClusters) ||
		!bundle.clusterListsEqual(oldPolicyStatus.UnknownComplianceClusters, newUnknownComplianceClusters) {
		oldPolicyStatus.CompliantClusters = newCompliantClusters
		oldPolicyStatus.NonCompliantClusters = newNonCompliantClusters
		oldPolicyStatus.UnknownComplianceClusters = newUnknownComplianceClusters
	}
}

func (bundle *ClustersPerPolicyBundle) clusterListsEqual(oldClusters []string, newClusters []string) bool {
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
