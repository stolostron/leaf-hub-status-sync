// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedclusters

import (
	"fmt"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	configv1 "github.com/open-cluster-management/hub-of-hubs-data-types/apis/config/v1"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	clusterStatusSyncLogName       = "clusters-status-sync"
	managedClusterCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/managed-cluster-cleanup"
)

// AddClustersStatusController adds managed clusters status controller to the manager.
func AddClustersStatusController(mgr ctrl.Manager, transportObj transport.Transport, leafHubName string,
	incarnation uint64, _ *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &clusterv1.ManagedCluster{} }
	transportBundleKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.ManagedClustersMsgKey)

	transportRetryChan := make(chan *transport.Message)

	// create bundle delivery registration
	clustersStatusDeliveryRegistration := transport.NewBundleDeliveryRegistration(transportRetryChan, nil)

	bundleCollection := []*generic.BundleCollectionEntry{ // single bundle for managed clusters
		generic.NewBundleCollectionEntry(transportBundleKey, bundle.NewGenericStatusBundle(leafHubName,
			incarnation, nil),
			clustersStatusDeliveryRegistration), // at this point send all managed clusters even if aggregation-
		// level is minimal
	}
	// register in transport
	transportObj.Register(transportBundleKey, clustersStatusDeliveryRegistration)

	if err := generic.NewGenericStatusSyncController(mgr, clusterStatusSyncLogName, transportObj,
		managedClusterCleanupFinalizer, bundleCollection, createObjFunction, nil, transportRetryChan,
		syncIntervalsData.GetManagerClusters); err != nil {
		return fmt.Errorf("failed to add managed clusters controller to the manager - %w", err)
	}

	return nil
}
