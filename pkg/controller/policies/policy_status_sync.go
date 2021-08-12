// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package policies

import (
	"fmt"
	"time"

	policiesv1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	configv1 "github.com/open-cluster-management/hub-of-hubs-data-types/apis/config/v1"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/generic"
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
func AddPoliciesStatusController(mgr ctrl.Manager, transport transport.Transport, syncInterval time.Duration,
	leafHubName string, hubOfHubsConfig *configv1.Config) error {
	createObjFunction := func() bundle.Object { return &policiesv1.Policy{} }

	// clusters per policy (base bundle)
	clustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.ClustersPerPolicyMsgKey)
	clustersPerPolicyBundle := bundle.NewClustersPerPolicyBundle(leafHubName, helpers.GetBundleGenerationFromTransport(
		transport, clustersPerPolicyTransportKey, datatypes.StatusBundle))

	// minimal compliance status bundle
	minComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.MinimalPolicyComplianceMsgKey)
	minComplianceStatusBundle := bundle.NewMinimalComplianceStatusBundle(leafHubName,
		helpers.GetBundleGenerationFromTransport(transport, minComplianceStatusTransportKey, datatypes.StatusBundle))

	// hybrid compliance status bundles & manager
	// - complete state bundle key
	completeComplianceStatusTransportKey := fmt.Sprintf("%s.%s",
		leafHubName, datatypes.PolicyCompleteComplianceMsgKey)
	// - delta state bundle key
	deltaComplianceStatusTransportKey := fmt.Sprintf("%s.%s",
		leafHubName, datatypes.PolicyDeltaComplianceMsgKey)

	fullStatusPredicate := func() bool { return hubOfHubsConfig.Spec.AggregationLevel == configv1.Full }
	minStatusPredicate := func() bool { return hubOfHubsConfig.Spec.AggregationLevel == configv1.Minimal }
	defaultDeliveryConsumer := func(int) {}

	// hybrid compliance status manager
	hybridComplianceStatusManager,
		completeComplianceBundle, deltaComplianceBundle,
		complianceBundleDeliveryConsumerFunc,
		completeBundlePred, deltaBundlePred := initHybridComplianceStatusManager(leafHubName,
		clustersPerPolicyBundle, fullStatusPredicate, fullStatusPredicate)

	defer hybridComplianceStatusManager.Stop()

	bundleCollection := []*generic.BundleCollectionEntry{ // multiple bundles for policy status
		generic.NewBundleCollectionEntry(clustersPerPolicyTransportKey, clustersPerPolicyBundle,
			fullStatusPredicate, defaultDeliveryConsumer),
		generic.NewBundleCollectionEntry(deltaComplianceStatusTransportKey,
			deltaComplianceBundle, deltaBundlePred, complianceBundleDeliveryConsumerFunc),
		generic.NewBundleCollectionEntry(completeComplianceStatusTransportKey,
			completeComplianceBundle, completeBundlePred, complianceBundleDeliveryConsumerFunc),
		generic.NewBundleCollectionEntry(minComplianceStatusTransportKey, minComplianceStatusBundle,
			minStatusPredicate, defaultDeliveryConsumer),
	} // IMPORTANT: delta-state bundle has to be placed before the complete-state bundle!

	hohNamespacePredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return meta.GetNamespace() == datatypes.HohSystemNamespace
	})
	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation)
	})

	// initialize policy status controller (contains multiple bundles)
	if err := generic.NewGenericStatusSyncController(mgr, policiesStatusSyncLog, transport, policyCleanupFinalizer,
		bundleCollection, createObjFunction, syncInterval,
		predicate.And(hohNamespacePredicate, ownerRefAnnotationPredicate)); err != nil {
		return fmt.Errorf("failed to add controller to the manager - %w", err)
	}

	return nil
}

// initHybridComplianceStatusManager starts a new instance of genericHybridStatusManager and returns:
// completeComplianceBundle - the complete compliance status hybrid bundle
// deltaComplianceBundle - the delta compliance status hybrid bundle
// complianceBundleDeliveryFunc - a function that manages sync mode based on delivery events
// completeBundlePred - a predicate that determines whether the completeComplianceBundle should be shipped
// deltaBundlePred - a predicate that determines whether the deltaComplianceBundle should be shipped
// All the returned elements are used inside bundleCollectionEntries.
func initHybridComplianceStatusManager(leafHubName string,
	completeComplianceBaseBundle bundle.Bundle, completeCompliancePred func() bool,
	deltaCompliancePred func() bool) (*generic.HybridStatusManager,
	bundle.HybridBundle, bundle.HybridBundle,
	func(int),
	func() bool, func() bool) {
	// policies map to serve as policies cache for delta bundles
	policiesMap := make(map[string]bool)
	// complete compliance status bundle
	completeComplianceStatusBundle := bundle.NewCompleteComplianceStatusBundle(leafHubName,
		completeComplianceBaseBundle, 0, policiesMap)
	// delta compliance status bundle
	deltaComplianceStatusBundle := bundle.NewDeltaComplianceStatusBundle(leafHubName,
		0, completeComplianceStatusBundle, policiesMap)

	// hybrid compliance status manager
	hybridComplianceStatusManager := generic.NewGenericHybridStatusManager(completeComplianceStatusBundle,
		deltaComplianceStatusBundle)
	// - delivery consumption func
	complianceBundleDeliveryConsumerFunc := hybridComplianceStatusManager.GenerateDeliveryConsumptionFunc()
	// - predicates
	completeBundlePred := hybridComplianceStatusManager.GenerateCompleteStateBundlePredicate(completeCompliancePred)
	deltaBundlePred := hybridComplianceStatusManager.GenerateDeltaStateBundlePredicate(deltaCompliancePred)

	hybridComplianceStatusManager.Start()

	return hybridComplianceStatusManager, completeComplianceStatusBundle, deltaComplianceStatusBundle,
		complianceBundleDeliveryConsumerFunc, completeBundlePred, deltaBundlePred
}
