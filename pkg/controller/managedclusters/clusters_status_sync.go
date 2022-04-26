// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedclusters

import (
	"fmt"

	datatypes "github.com/stolostron/hub-of-hubs-data-types"
	configv1 "github.com/stolostron/hub-of-hubs-data-types/apis/config/v1"
	"github.com/stolostron/leaf-hub-status-sync/pkg/bundle"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/stolostron/leaf-hub-status-sync/pkg/helpers"
	"github.com/stolostron/leaf-hub-status-sync/pkg/transport"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	clusterStatusSyncLogName          = "clusters-status-sync"
	managedClusterManagedByAnnotation = "hub-of-hubs.open-cluster-management.io/managed-by"
)

// AddClustersStatusController adds managed clusters status controller to the manager.
func AddClustersStatusController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &clusterv1.ManagedCluster{} }
	transportBundleKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.ManagedClustersMsgKey)
	manipulateObjFunc := func(object bundle.Object) {
		helpers.AddAnnotations(object, map[string]string{
			managedClusterManagedByAnnotation: leafHubName,
		})
	}

	bundleCollection := []*generic.BundleCollectionEntry{ // single bundle for managed clusters
		generic.NewBundleCollectionEntry(transportBundleKey, bundle.NewGenericStatusBundle(leafHubName, incarnation,
			manipulateObjFunc),
			func() bool { // bundle predicate
				return hubOfHubsConfig.Spec.AggregationLevel == configv1.Full ||
					hubOfHubsConfig.Spec.AggregationLevel == configv1.Minimal
			}), // at this point send all managed clusters even if aggregation level is minimal
	}

	if err := generic.NewGenericStatusSyncController(mgr, clusterStatusSyncLogName, transport, bundleCollection,
		createObjFunction, nil, syncIntervalsData.GetManagerClusters); err != nil {
		return fmt.Errorf("failed to add managed clusters controller to the manager - %w", err)
	}

	return nil
}
