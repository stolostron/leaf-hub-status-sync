package appsub

import (
	"fmt"

	datatypes "github.com/stolostron/hub-of-hubs-data-types"
	configv1 "github.com/stolostron/hub-of-hubs-data-types/apis/config/v1"
	"github.com/stolostron/leaf-hub-status-sync/pkg/bundle"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/stolostron/leaf-hub-status-sync/pkg/transport"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	subscriptionReportSyncLog          = "subscription-reports-sync"
	subscriptionReportCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/subscriptionreport-cleanup"
)

// AddSubscriptionReportsController adds subscriptions-report controller to the manager.
func AddSubscriptionReportsController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &appsv1alpha1.SubscriptionReport{} }
	subscriptionReportTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.SubscriptionReportMsgKey)

	if err := addResourceController(mgr, transport, subscriptionReportTransportKey, leafHubName, createObjFunction,
		cleanSubscriptionReport, subscriptionReportSyncLog, subscriptionReportCleanupFinalizer, incarnation,
		hubOfHubsConfig, nil, syncIntervalsData); err != nil {
		return fmt.Errorf("failed to create subscription-reports controller - %w", err)
	}

	return nil
}

func cleanSubscriptionReport(object bundle.Object) {
	if _, ok := object.(*appsv1alpha1.SubscriptionReport); !ok {
		panic("Wrong instance passed to clean subscription-report function, not a subscription-report")
	}
}
