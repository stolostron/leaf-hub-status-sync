package apps

import (
	"fmt"

	datatypes "github.com/stolostron/hub-of-hubs-data-types"
	configv1 "github.com/stolostron/hub-of-hubs-data-types/apis/config/v1"
	"github.com/stolostron/leaf-hub-status-sync/pkg/bundle"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/stolostron/leaf-hub-status-sync/pkg/transport"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	subscriptionStatusSyncLog = "subscriptions-statuses-sync"
)

// AddSubscriptionStatusesController adds subscription-status controller to the manager.
func AddSubscriptionStatusesController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	incarnation uint64, _ *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &appsv1alpha1.SubscriptionStatus{} }

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(fmt.Sprintf("%s.%s", leafHubName, datatypes.SubscriptionStatusMsgKey),
			bundle.NewGenericStatusBundle(leafHubName, incarnation, nil),
			func() bool { return true }),
	} // bundle predicate - always send subscription status.

	if err := generic.NewGenericStatusSyncController(mgr, subscriptionStatusSyncLog, transport, bundleCollection,
		createObjFunction, nil, syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add subscription statuses controller to the manager - %w", err)
	}

	return nil
}
