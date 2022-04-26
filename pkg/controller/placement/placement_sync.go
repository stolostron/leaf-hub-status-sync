package placement

import (
	"fmt"

	datatypes "github.com/stolostron/hub-of-hubs-data-types"
	configv1 "github.com/stolostron/hub-of-hubs-data-types/apis/config/v1"
	"github.com/stolostron/leaf-hub-status-sync/pkg/bundle"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/stolostron/leaf-hub-status-sync/pkg/helpers"
	"github.com/stolostron/leaf-hub-status-sync/pkg/transport"
	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	placementSyncLog = "placement-sync"
)

// AddPlacementsController adds placement controller to the manager.
func AddPlacementsController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	incarnation uint64, _ *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &clustersv1beta1.Placement{} }

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(fmt.Sprintf("%s.%s", leafHubName, datatypes.PlacementMsgKey),
			bundle.NewGenericStatusBundle(leafHubName, incarnation, cleanPlacement),
			func() bool { return true }),
	} // bundle predicate - always send placements.

	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return helpers.HasAnnotation(object, datatypes.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewGenericStatusSyncController(mgr, placementSyncLog, transport, bundleCollection,
		createObjFunction, ownerRefAnnotationPredicate, syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add placements controller to the manager - %w", err)
	}

	return nil
}

func cleanPlacement(object bundle.Object) {
	placement, ok := object.(*clustersv1beta1.Placement)
	if !ok {
		panic("Wrong instance passed to clean placement function, not a placement")
	}
	// clean spec. no need for it.
	placement.Spec = clustersv1beta1.PlacementSpec{}
}
