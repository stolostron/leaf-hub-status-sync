// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package subscriptions

import (
	"fmt"

	datatypes "github.com/stolostron/hub-of-hubs-data-types"
	configv1 "github.com/stolostron/hub-of-hubs-data-types/apis/config/v1"
	"github.com/stolostron/leaf-hub-status-sync/pkg/bundle"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/stolostron/leaf-hub-status-sync/pkg/helpers"
	"github.com/stolostron/leaf-hub-status-sync/pkg/transport"
	subscriptionsv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	subscriptionStatusSyncLog = "subscriptions-status-sync"
)

// AddSubscriptionStatusController adds subscriptions status controller to the manager.
func AddSubscriptionStatusController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &subscriptionsv1.Subscription{} }
	subscriptionTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.SubscriptionStatusMsgKey)

	bundleCollection := []*generic.BundleCollectionEntry{generic.NewBundleCollectionEntry(subscriptionTransportKey,
		bundle.NewGenericStatusBundle(leafHubName, incarnation, cleanSubscription),
		func() bool { // bundle predicate
			return true
		})}

	isGlobalSubscription := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return helpers.HasAnnotation(object, datatypes.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewGenericStatusSyncController(mgr, subscriptionStatusSyncLog, transport, bundleCollection,
		createObjFunction, isGlobalSubscription, syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add subscriptions controller to the manager - %w", err)
	}

	return nil
}

func cleanSubscription(object bundle.Object) {
	placement, ok := object.(*subscriptionsv1.Subscription)
	if !ok {
		panic("Wrong instance passed to clean subscription function, not a subscription")
	}

	placement.Spec = subscriptionsv1.SubscriptionSpec{}
}
