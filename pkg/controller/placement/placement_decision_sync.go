package placement

import (
	"fmt"

	datatypes "github.com/stolostron/hub-of-hubs-data-types"
	configv1 "github.com/stolostron/hub-of-hubs-data-types/apis/config/v1"
	"github.com/stolostron/leaf-hub-status-sync/pkg/bundle"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/stolostron/leaf-hub-status-sync/pkg/transport"
	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	placementDecisionsSyncLog = "placement-decisions-sync"
)

// AddPlacementDecisionsController adds placement-decision controller to the manager.
func AddPlacementDecisionsController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	incarnation uint64, _ *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &clustersv1beta1.PlacementDecision{} }

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(fmt.Sprintf("%s.%s", leafHubName, datatypes.PlacementDecisionMsgKey),
			bundle.NewGenericStatusBundle(leafHubName, incarnation, nil),
			func() bool { return true }),
	} // bundle predicate - always send placement decision.

	if err := generic.NewGenericStatusSyncController(mgr, placementDecisionsSyncLog, transport, bundleCollection,
		createObjFunction, nil, syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add placement decisions controller to the manager - %w", err)
	}

	return nil
}
