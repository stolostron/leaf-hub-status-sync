package appsub

import (
	"fmt"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	configv1 "github.com/stolostron/hub-of-hubs-data-types/apis/config/v1"
	"github.com/stolostron/leaf-hub-status-sync/pkg/bundle"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/stolostron/leaf-hub-status-sync/pkg/transport"
	ctrl "sigs.k8s.io/controller-runtime"
)

func addResourceController(mgr ctrl.Manager, transport transport.Transport, resourceTransportKey string,
	leafHubName string, createObjFunction func() bundle.Object, cleanResourceFunc func(object bundle.Object),
	syncLog string, resourceCleanupFinalizer string, incarnation uint64, hubOfHubsConfig *configv1.Config,
	predicate predicate.Predicate, syncIntervalsData *syncintervals.SyncIntervals) error {
	bundleCollection := []*generic.BundleCollectionEntry{generic.NewBundleCollectionEntry(resourceTransportKey,
		bundle.NewGenericStatusBundle(leafHubName, incarnation, cleanResourceFunc),
		func() bool { // bundle predicate
			return hubOfHubsConfig.Spec.AggregationLevel == configv1.Full ||
				hubOfHubsConfig.Spec.AggregationLevel == configv1.Minimal
		})}

	if err := generic.NewGenericStatusSyncController(mgr, syncLog, transport, resourceCleanupFinalizer,
		bundleCollection, createObjFunction, predicate, syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add controller to the manager - %w", err)
	}

	return nil
}
