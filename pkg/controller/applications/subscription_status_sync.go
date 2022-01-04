// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package applications

import (
	"fmt"

	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	configv1 "github.com/open-cluster-management/hub-of-hubs-data-types/apis/config/v1"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	subv1 "github.com/open-cluster-management/multicloud-operators-subscription/pkg/apis/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	subscriptionStatusSyncLog    = "subscriptions-status-sync"
	subscriptionCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/subscription-cleanup"
)

// AddSubscriptionStatusController adds subscription status controller to the manager.
func AddSubscriptionStatusController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &subv1.Subscription{} }

	subscriptionTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.SubscriptionStatusMsgKey)
	subscriptionBundle := generic.NewBundleCollectionEntry(subscriptionTransportKey,
		bundle.NewGenericStatusBundle(leafHubName, incarnation, cleanSubscriptionFunction),
		func() bool { // bundle predicate
			return true
		})

	bundleCollection := []*generic.BundleCollectionEntry{subscriptionBundle}

	isGlobalSubscription := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation)
	isGlobalSubscription := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return helpers.HasAnnotation(object, datatypes.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewGenericStatusSyncController(mgr, subscriptionStatusSyncLog, transport,
		subscriptionCleanupFinalizer, bundleCollection, createObjFunction, isGlobalSubscription,
		syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed adding subscription controller - %w", err)
	}

	return nil
}

func cleanSubscriptionFunction(object bundle.Object) {
	placement, ok := object.(*subv1.Subscription)
	if !ok {
		panic("Wrong instance passed to clean placement rule function, not subv1.subscription")
	}

	placement.Spec = subv1.SubscriptionSpec{}
}
