// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package policies

import (
	"fmt"
	policiesv1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/predicate"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

const (
	policiesStatusSyncLog       = "policies-status-sync"
	policiesFinalizerCleanerLog = "policies-finalizer-cleanup"
	policyCleanupFinalizer      = "hub-of-hubs.open-cluster-management.io/policy-cleanup"
)

func AddPoliciesStatusController(mgr ctrl.Manager, transport transport.Transport, syncInterval time.Duration,
	leafHubName string) error {
	createObjFunction := func() bundle.Object { return &policiesv1.Policy{} }
	clustersPerPolicyBundle := bundle.NewClustersPerPolicyBundle(leafHubName)
	complianceStatueBundle := bundle.NewComplianceStatusBundle(leafHubName, clustersPerPolicyBundle)
	bundleCollection := []*generic.BundleCollectionEntry{ // multiple bundles for policy status
		generic.NewBundleCollectionEntry(fmt.Sprintf("%s.%s", leafHubName, datatypes.ClustersPerPolicyMsgKey),
			clustersPerPolicyBundle),
		generic.NewBundleCollectionEntry(fmt.Sprintf("%s.%s", leafHubName, datatypes.PolicyComplianceMsgKey),
			complianceStatueBundle),
	}
	// initialize policy status controller (sends two bundles, list of clusters per policy and compliance status)
	err := generic.NewGenericStatusSyncController(mgr, policiesStatusSyncLog, transport, policyCleanupFinalizer,
		bundleCollection, createObjFunction, syncInterval, true, &predicate.HohNamespacePredicate{})
	if err != nil {
		return err
	}
	// initialize policy finalizer cleaner from policies that are not in hoh-system namespace (replicated policies)
	if err = newPolicyFinalizerCleanerController(mgr, policiesFinalizerCleanerLog, policyCleanupFinalizer); err != nil {
		return err
	}

	return nil
}
