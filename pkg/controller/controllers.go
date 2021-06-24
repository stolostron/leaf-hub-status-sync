// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"
	clustersv1 "github.com/open-cluster-management/api/cluster/v1"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

// AddToScheme adds all Resources to the Scheme.
func AddToScheme(s *runtime.Scheme) error {
	// add cluster scheme
	if err := clustersv1.Install(s); err != nil {
		return fmt.Errorf("failed to add scheme: %w", err)
	}
	return nil
}

func AddControllers(mgr ctrl.Manager, transportImpl transport.Transport, syncInterval time.Duration,
	leafHubName string) error {
	addControllerFunctions := []func(ctrl.Manager, transport.Transport, time.Duration, string) error{
		addClustersStatusController,
	}

	for _, addControllerFunction := range addControllerFunctions {
		if err := addControllerFunction(mgr, transportImpl, syncInterval, leafHubName); err != nil {
			return fmt.Errorf("failed to add controller: %w", err)
		}
	}

	return nil
}
