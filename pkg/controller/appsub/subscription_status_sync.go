// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

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
	subscriptionStatusSyncLog          = "subscriptions-statuses-sync"
	subscriptionStatusCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/subscriptionstatus-cleanup"
)

// AddSubscriptionStatusesController adds subscription-status controller to the manager.
func AddSubscriptionStatusesController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &appsv1alpha1.SubscriptionStatus{} }
	subscriptionStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.SubscriptionStatusMsgKey)

	if err := addResourceController(mgr, transport, subscriptionStatusTransportKey, leafHubName, createObjFunction,
		cleanSubscriptionStatus, subscriptionStatusSyncLog, subscriptionStatusCleanupFinalizer, incarnation,
		hubOfHubsConfig, nil, syncIntervalsData); err != nil {
		return fmt.Errorf("failed to create subscription-statuses controller - %w", err)
	}

	return nil
}

func cleanSubscriptionStatus(object bundle.Object) {
	if _, ok := object.(*appsv1alpha1.SubscriptionStatus); !ok {
		panic("Wrong instance passed to clean subscription-status function, not a subscription-status")
	}
}
