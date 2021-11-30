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
func AddLocalPlacementRulesController(mgr ctrl.Manager, transportObj transport.Transport, leafHubName string,
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunc := func() bundle.Object { return &placementrulesv1.PlacementRule{} }
	transportRetryChan := make(chan *transport.Message)

	localPlacementRuleTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.LocalPlacementRulesMsgKey)

	// create bundle delivery registration
	localPlacementRuleDeliveryRegistration := transport.NewBundleDeliveryRegistration(transportRetryChan, nil)

	// add BeforeDeliveryAttempt condition
	localPlacementRuleDeliveryRegistration.AddCondition(transport.BeforeDeliveryAttempt, transport.ArgTypeNone,
		func(interface{}) bool {
			return hubOfHubsConfig.Spec.EnableLocalPolicies
		})

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(localPlacementRuleTransportKey,
			bundle.NewGenericStatusBundle(leafHubName, incarnation, cleanPlacementRuleFunc),
			localPlacementRuleDeliveryRegistration),
	}
	// register in transport
	transportObj.Register(localPlacementRuleTransportKey, localPlacementRuleDeliveryRegistration)

	// controller predicate
	localPlacementRulePredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return !helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewGenericStatusSyncController(mgr, localPlacementRuleStatusSyncLog, transportObj,
		localPlacementRuleCleanupFinalizer, bundleCollection, createObjFunc, localPlacementRulePredicate,
		transportRetryChan, syncIntervalsData.GetPolicies); err != nil {
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
