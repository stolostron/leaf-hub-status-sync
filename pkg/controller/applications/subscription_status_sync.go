// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package applications

import (
	"fmt"
	"time"

	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	configv1 "github.com/open-cluster-management/hub-of-hubs-data-types/apis/config/v1"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	subv1 "github.com/open-cluster-management/multicloud-operators-subscription/pkg/apis/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	subscriptionStatusSyncLog    = "subscriptions-status-sync"
	subscriptionCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/subscription-cleanup"

	// TODO: Move to datatypes repository, here it's temporary.
	subscriptionStatusMsgKey = "subscriptionStatus"
)

// AddSubscriptionStatusController adds subscription status controller to the manager.
func AddSubscriptionStatusController(mgr ctrl.Manager, transport transport.Transport, syncInterval time.Duration,
	leafHubName string, hubOfHubsConfig *configv1.Config) error {
	createObjFunction := func() bundle.Object { return &subv1.Subscription{} }

	// Generating a new placement rule bundle.
	cleanFunc :=
		func(object bundle.Object) (bundle.Object, bool) {
			subscription, ok := object.(*subv1.Subscription)
			if !ok {
				return nil, ok
			}

			subscription.Spec = subv1.SubscriptionSpec{}

			return subscription, true
		}
	subscriptionTransportKey := fmt.Sprintf("%s.%s", leafHubName, subscriptionStatusMsgKey)
	subscriptionBundle := generic.NewBundleCollectionEntry(subscriptionTransportKey,
		bundle.NewGenericStatusBundle(leafHubName,
			helpers.GetBundleGenerationFromTransport(transport, subscriptionTransportKey, datatypes.StatusBundle),
			cleanFunc),
		func() bool { // bundle predicate
			return true
		})

	bundleCollection := []*generic.BundleCollectionEntry{subscriptionBundle}

	isGlobalSubscription := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewGenericStatusSyncController(mgr, subscriptionStatusSyncLog, transport,
		subscriptionCleanupFinalizer, bundleCollection, createObjFunction, syncInterval,
		isGlobalSubscription); err != nil {
		return fmt.Errorf("failed adding subscription controller - %w", err)
	}

	return nil
}
