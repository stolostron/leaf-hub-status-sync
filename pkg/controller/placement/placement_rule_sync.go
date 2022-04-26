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
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	placementRuleSyncLog = "placement-rules-sync"
)

// AddPlacementRulesController adds placement-rule controller to the manager.
func AddPlacementRulesController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	incarnation uint64, _ *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &placementrulesv1.PlacementRule{} }

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(fmt.Sprintf("%s.%s", leafHubName, datatypes.PlacementRuleMsgKey),
			bundle.NewGenericStatusBundle(leafHubName, incarnation, cleanPlacementRule),
			func() bool { return true }),
	} // bundle predicate - always send placement rules.

	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return helpers.HasAnnotation(object, datatypes.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewGenericStatusSyncController(mgr, placementRuleSyncLog, transport, bundleCollection,
		createObjFunction, ownerRefAnnotationPredicate, syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add placement rules controller to the manager - %w", err)
	}

	return nil
}

func cleanPlacementRule(object bundle.Object) {
	placementrule, ok := object.(*placementrulesv1.PlacementRule)
	if !ok {
		panic("Wrong instance passed to clean placement-rule function, not a placement-rule")
	}
	// clean spec. no need for it.
	placementrule.Spec = placementrulesv1.PlacementRuleSpec{}
}
