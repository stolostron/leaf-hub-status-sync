// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

func addClustersStatusController(mgr ctrl.Manager, transport transport.Transport, syncInterval time.Duration,
	leafHubId string) error {
	createObjFunction := func() bundle.Object { return &clusterv1.ManagedCluster{} }
	finalizerName := "hub-of-hubs.open-cluster-management.io/managed-cluster-cleanup"
	return newGenericStatusSyncController(mgr, "clusters-status-sync", transport, finalizerName,
		datatypes.ManagedClustersMsgKey, createObjFunction, syncInterval, leafHubId)
}
