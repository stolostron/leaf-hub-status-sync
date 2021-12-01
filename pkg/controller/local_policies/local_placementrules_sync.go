package localpolicies

import (
	"fmt"

	placementrulesv1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/apps/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	configv1 "github.com/open-cluster-management/hub-of-hubs-data-types/apis/config/v1"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	bundleregistration "github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport/bundle-registration"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	localPlacementRuleStatusSyncLog    = "local-placement-rule-status-sync"
	localPlacementRuleCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/local-placement-rule-cleanup"
)

// AddLocalPlacementRulesController adds a new local placement rules controller.
func AddLocalPlacementRulesController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunc := func() bundle.Object { return &placementrulesv1.PlacementRule{} }

	localPlacementRuleTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.LocalPlacementRulesMsgKey)

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(bundle.NewGenericStatusBundle(leafHubName, incarnation,
			cleanPlacementRuleFunc),
			func() bool { // bundle predicate
				return hubOfHubsConfig.Spec.EnableLocalPolicies
			},
			bundleregistration.NewFullStateBundleRegistration(localPlacementRuleTransportKey)),
	}
	// controller predicate
	localPlacementRulePredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return !helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewGenericStatusSyncController(mgr, localPlacementRuleStatusSyncLog, transport,
		localPlacementRuleCleanupFinalizer, bundleCollection, createObjFunc, localPlacementRulePredicate,
		syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add local placement rules controller to the manager - %w", err)
	}

	return nil
}

func cleanPlacementRuleFunc(object bundle.Object) {
	placement, ok := object.(*placementrulesv1.PlacementRule)
	if !ok {
		panic("Wrong instance passed to clean placement rule function, not appsv1.PlacementRule")
	}

	placement.Status = placementrulesv1.PlacementRuleStatus{}
}
