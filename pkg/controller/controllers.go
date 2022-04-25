// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	clustersv1 "github.com/open-cluster-management/api/cluster/v1"
	policiesv1 "github.com/open-cluster-management/governance-policy-propagator/api/v1"
	configv1 "github.com/stolostron/hub-of-hubs-data-types/apis/config/v1"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/appsub"
	configCtrl "github.com/stolostron/leaf-hub-status-sync/pkg/controller/config"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/controlinfo"
	localpolicies "github.com/stolostron/leaf-hub-status-sync/pkg/controller/local_policies"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/managedclusters"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/policies"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/stolostron/leaf-hub-status-sync/pkg/transport"
	"k8s.io/apimachinery/pkg/runtime"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// AddToScheme adds all Resources to the Scheme.
func AddToScheme(runtimeScheme *runtime.Scheme) error {
	// add cluster scheme
	if err := clustersv1.Install(runtimeScheme); err != nil {
		return fmt.Errorf("failed to add scheme: %w", err)
	}

	schemeBuilders := []*scheme.Builder{
		policiesv1.SchemeBuilder, configv1.SchemeBuilder, placementrulesv1.SchemeBuilder, appsv1alpha1.SchemeBuilder,
	} // add schemes

	for _, schemeBuilder := range schemeBuilders {
		if err := schemeBuilder.AddToScheme(runtimeScheme); err != nil {
			return fmt.Errorf("failed to add scheme: %w", err)
		}
	}

	return nil
}

// AddControllers adds all the controllers to the Manager.
func AddControllers(mgr ctrl.Manager, transportImpl transport.Transport, leafHubName string, incarnation uint64) error {
	config := &configv1.Config{}
	syncIntervalsData := syncintervals.NewSyncIntervals()

	if err := configCtrl.AddConfigController(mgr, config); err != nil {
		return fmt.Errorf("failed to add controller: %w", err)
	}

	if err := syncintervals.AddSyncIntervalsController(mgr, syncIntervalsData); err != nil {
		return fmt.Errorf("failed to add controller: %w", err)
	}

	addControllerFunctions := []func(ctrl.Manager, transport.Transport, string, uint64, *configv1.Config,
		*syncintervals.SyncIntervals) error{
		managedclusters.AddClustersStatusController,
		policies.AddPoliciesStatusController,
		appsub.AddPlacementRulesController,
		appsub.AddSubscriptionStatusesController,
		appsub.AddSubscriptionReportsController,
		localpolicies.AddLocalPoliciesController,
		localpolicies.AddLocalPlacementRulesController,
		controlinfo.AddControlInfoController,
	}

	for _, addControllerFunction := range addControllerFunctions {
		if err := addControllerFunction(mgr, transportImpl, leafHubName, incarnation, config,
			syncIntervalsData); err != nil {
			return fmt.Errorf("failed to add controller: %w", err)
		}
	}

	return nil
}
