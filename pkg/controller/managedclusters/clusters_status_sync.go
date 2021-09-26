// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedclusters

import (
	"fmt"
	"time"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	configv1 "github.com/open-cluster-management/hub-of-hubs-data-types/apis/config/v1"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	clusterStatusSyncLogName       = "clusters-status-sync"
	managedClusterCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/managed-cluster-cleanup"
)

// AddClustersStatusController adds managed clusters status controller to the manager.
func AddClustersStatusController(mgr ctrl.Manager, transportObj transport.Transport, syncInterval time.Duration,
	leafHubName string, incarnation uint64, _ *configv1.Config) error {
	createObjFunction := func() bundle.Object { return &clusterv1.ManagedCluster{} }
	transportBundleKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.ManagedClustersMsgKey)

	transportRetryChan := make(chan *transport.Message)

	// create bundle delivery registration and register conditions
	clustersStatusDeliveryRegistration := transport.NewBundleDeliveryRegistration(0, transportRetryChan, nil)
	// add condition on pre-attempt
	clustersStatusDeliveryRegistration.AddCondition(transport.BeforeDeliveryAttempt,
		transport.ArgTypeBundleGeneration, func(generation interface{}) bool {
			return generation.(uint64) > clustersStatusDeliveryRegistration.GetLastSentGeneration()
		})
	// add condition on pre-retry-attempt
	clustersStatusDeliveryRegistration.AddCondition(transport.BeforeDeliveryRetry,
		transport.ArgTypeBundleGeneration, func(generation interface{}) bool {
			return generation.(uint64) == clustersStatusDeliveryRegistration.GetLastSentGeneration()
		})

	bundleCollection := []*generic.BundleCollectionEntry{ // single bundle for managed clusters
		generic.NewBundleCollectionEntry(transportBundleKey,
			bundle.NewGenericStatusBundle(leafHubName, incarnation, 0),
			clustersStatusDeliveryRegistration), // at this point send all managed clusters even if aggregation level is minimal
	}

	if err := generic.NewGenericStatusSyncController(mgr, clusterStatusSyncLogName, incarnation, transportObj,
		managedClusterCleanupFinalizer, bundleCollection, createObjFunction, syncInterval, transportRetryChan,
		nil); err != nil {
		return fmt.Errorf("failed to add controller to the manager - %w", err)
	}

	return nil
}
