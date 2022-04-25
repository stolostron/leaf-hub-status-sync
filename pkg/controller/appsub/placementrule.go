package appsub

import (
	"fmt"
	"github.com/stolostron/leaf-hub-status-sync/pkg/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	datatypes "github.com/stolostron/hub-of-hubs-data-types"
	configv1 "github.com/stolostron/hub-of-hubs-data-types/apis/config/v1"
	"github.com/stolostron/leaf-hub-status-sync/pkg/bundle"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/stolostron/leaf-hub-status-sync/pkg/transport"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	placementRuleReportSyncLog          = "placement-rule-reports-sync"
	placementRuleReportCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/placementrule-cleanup"
)

// AddPlacementRulesController adds placement-rule controller to the manager.
func AddPlacementRulesController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &placementrulesv1.PlacementRule{} }
	subscriptionReportTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.PlacementRuleMsgKey)

	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return helpers.HasAnnotation(object, datatypes.OriginOwnerReferenceAnnotation)
	})

	if err := addResourceController(mgr, transport, subscriptionReportTransportKey, leafHubName, createObjFunction,
		cleanPlacementRule, placementRuleReportSyncLog, placementRuleReportCleanupFinalizer, incarnation,
		hubOfHubsConfig, ownerRefAnnotationPredicate, syncIntervalsData); err != nil {
		return fmt.Errorf("failed to create placement-rules controller - %w", err)
	}

	return nil
}

func cleanPlacementRule(object bundle.Object) {
	if _, ok := object.(*placementrulesv1.PlacementRule); !ok {
		panic("Wrong instance passed to clean placement-rule function, not a placement-rule")
	}
}
