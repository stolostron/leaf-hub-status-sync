package localpolicies

import (
	"fmt"

	placementrulev1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/apps/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	configv1 "github.com/open-cluster-management/hub-of-hubs-data-types/apis/config/v1"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	localPlacementRuleStatusSyncLog    = "placement-rule-status-sync"
	localPlacementRuleCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/local-placement-rule-cleanup"
)

// AddLocalPlacementRuleController This function adds a new local placement rule controller.
func AddLocalPlacementRuleController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunc := func() bundle.Object { return &placementrulev1.PlacementRule{} }

	localPlacementRuleTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.LocalPlacementRulesMsgKey)

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(localPlacementRuleTransportKey,
			bundle.NewGenericStatusBundle(leafHubName,
				helpers.GetGenerationFromTransport(transport, localPlacementRuleTransportKey, datatypes.StatusBundle),
				cleanPlacementRuleFunc),
			func() bool { // bundle predicate
				return hubOfHubsConfig.Spec.EnableLocalPolicies
			}),
	}

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
	placement, ok := object.(*placementrulev1.PlacementRule)
	if !ok {
		panic("Wrong instance passed to clean placement rule function, not appsv1.PlacementRule")
	}

	placement.Status = placementrulev1.PlacementRuleStatus{}
}
