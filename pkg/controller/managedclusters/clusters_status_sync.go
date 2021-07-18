// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedclusters

import (
	"fmt"
	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

const (
	clusterStatusSyncLogName       = "clusters-status-sync"
	managedClusterCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/managed-cluster-cleanup"
)

func AddClustersStatusController(mgr ctrl.Manager, transport transport.Transport, syncInterval time.Duration,
	leafHubName string) error {
	createObjFunction := func() bundle.Object { return &clusterv1.ManagedCluster{} }
	transportBundleKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.ManagedClustersMsgKey)
	bundleCollection := []*generic.BundleCollectionEntry{ //single bundle for managed clusters
		generic.NewBundleCollectionEntry(transportBundleKey, bundle.NewGenericStatusBundle(leafHubName,
			helpers.GetBundleGenerationFromTransport(transport, transportBundleKey, datatypes.StatusBundle))),
	}
	return generic.NewGenericStatusSyncController(mgr, clusterStatusSyncLogName, transport,
		managedClusterCleanupFinalizer, bundleCollection, createObjFunction, syncInterval, false,
		nil)
}
