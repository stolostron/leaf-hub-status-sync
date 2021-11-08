// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package policies

import (
	"fmt"

	policiesv1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	configv1 "github.com/open-cluster-management/hub-of-hubs-data-types/apis/config/v1"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	policiesStatusSyncLog  = "policies-status-sync"
	policyCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/policy-cleanup"
)

// AddPoliciesStatusController adds policies status controller to the manager.
func AddPoliciesStatusController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &policiesv1.Policy{} }

	// clusters per policy (base bundle)
	clustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.ClustersPerPolicyMsgKey)
	clustersPerPolicyBundle := bundle.NewClustersPerPolicyBundle(leafHubName, helpers.GetBundleGenerationFromTransport(
		transport, clustersPerPolicyTransportKey, datatypes.StatusBundle))

	// compliance status bundle
	complianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.PolicyComplianceMsgKey)
	complianceStatusBundle := bundle.NewComplianceStatusBundle(leafHubName, clustersPerPolicyBundle,
		helpers.GetBundleGenerationFromTransport(transport, complianceStatusTransportKey, datatypes.StatusBundle))

	// minimal compliance status bundle
	minComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.MinimalPolicyComplianceMsgKey)
	minComplianceStatusBundle := bundle.NewMinimalComplianceStatusBundle(leafHubName,
		helpers.GetBundleGenerationFromTransport(transport, minComplianceStatusTransportKey, datatypes.StatusBundle))

	fullStatusPredicate := func() bool { return hubOfHubsConfig.Spec.AggregationLevel == configv1.Full }
	minStatusPredicate := func() bool { return hubOfHubsConfig.Spec.AggregationLevel == configv1.Minimal }

	bundleCollection := []*generic.BundleCollectionEntry{ // multiple bundles for policy status
		generic.NewBundleCollectionEntry(clustersPerPolicyTransportKey, clustersPerPolicyBundle, fullStatusPredicate),
		generic.NewBundleCollectionEntry(complianceStatusTransportKey, complianceStatusBundle, fullStatusPredicate),
		generic.NewBundleCollectionEntry(minComplianceStatusTransportKey, minComplianceStatusBundle,
			minStatusPredicate),
	}

	hohNamespacePredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return meta.GetNamespace() == datatypes.HohSystemNamespace
	})
	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation)
	})

	// initialize policy status controller (contains multiple bundles)
	if err := generic.NewGenericStatusSyncController(mgr, policiesStatusSyncLog, transport, policyCleanupFinalizer,
		bundleCollection, createObjFunction, predicate.And(hohNamespacePredicate, ownerRefAnnotationPredicate),
		syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add controller to the manager - %w", err)
	}

	return nil
}
