// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	clustersv1 "github.com/open-cluster-management/api/cluster/v1"
	policiesv1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	configv1 "github.com/open-cluster-management/hub-of-hubs-data-types/apis/config/v1"
	configCtrl "github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/config"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/controlinfo"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/managedclusters"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/policies"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// AddToScheme adds all Resources to the Scheme.
func AddToScheme(s *runtime.Scheme) error {
	// add cluster scheme
	if err := clustersv1.Install(s); err != nil {
		return fmt.Errorf("failed to add scheme: %w", err)
	}

	schemeBuilders := []*scheme.Builder{policiesv1.SchemeBuilder, configv1.SchemeBuilder} // add schemes

	for _, schemeBuilder := range schemeBuilders {
		if err := schemeBuilder.AddToScheme(s); err != nil {
			return fmt.Errorf("failed to add scheme: %w", err)
		}
	}

	return nil
}

// AddControllers adds all the controllers to the Manager.
func AddControllers(mgr ctrl.Manager, transportImpl transport.Transport, leafHubName string) error {
	config := &configv1.Config{}
	syncIntervalsData := syncintervals.NewSyncIntervals()

	if err := configCtrl.AddConfigController(mgr, config); err != nil {
		return fmt.Errorf("failed to add controller: %w", err)
	}

	if err := syncintervals.AddSyncIntervalsController(mgr, syncIntervalsData); err != nil {
		return fmt.Errorf("failed to add controller: %w", err)
	}

	addControllerFunctions := []func(ctrl.Manager, transport.Transport, string, *configv1.Config,
		*syncintervals.SyncIntervals) error{
		managedclusters.AddClustersStatusController, policies.AddPoliciesStatusController,
		controlinfo.AddControlInfoController,
	}

	for _, addControllerFunction := range addControllerFunctions {
		if err := addControllerFunction(mgr, transportImpl, leafHubName, config, syncIntervalsData); err != nil {
			return fmt.Errorf("failed to add controller: %w", err)
		}
	}

	return nil
}
