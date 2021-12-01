// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package policies

import (
	"fmt"

	policiesv1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	configv1 "github.com/open-cluster-management/hub-of-hubs-data-types/apis/config/v1"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	compliancestatus "github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle/hybrid-bundle/compliance-status"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	bundleregistration "github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport/bundle-registration"
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
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &policiesv1.Policy{} }
	bundleCollection := createBundleCollection(leafHubName, incarnation, hubOfHubsConfig)

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
		return fmt.Errorf("failed to add policies controller to the manager - %w", err)
	}

	return nil
}

func createBundleCollection(leafHubName string, incarnation uint64,
	hubOfHubsConfig *configv1.Config) []*generic.BundleCollectionEntry {
	// clusters per policy (base bundle)
	clustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.ClustersPerPolicyMsgKey)
	clustersPerPolicyBundle := bundle.NewClustersPerPolicyBundle(leafHubName, incarnation, extractPolicyID)

	// complete compliance status bundle
	completeComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		datatypes.PolicyCompleteComplianceMsgKey)
	completeComplianceStatusBundle := compliancestatus.NewCompleteComplianceStatusBundle(leafHubName,
		clustersPerPolicyBundle, incarnation, extractPolicyID)

	// minimal compliance status bundle
	minimalComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		datatypes.MinimalPolicyComplianceMsgKey)
	minimalComplianceStatusBundle := bundle.NewMinimalComplianceStatusBundle(leafHubName, incarnation)

	fullStatusPredicate := func() bool { return hubOfHubsConfig.Spec.AggregationLevel == configv1.Full }
	minimalStatusPredicate := func() bool { return hubOfHubsConfig.Spec.AggregationLevel == configv1.Minimal }

	// no need to send in the same cycle both clusters per policy and compliance. if CpP was sent, don't send compliance
	return []*generic.BundleCollectionEntry{ // multiple bundles for policy status
		generic.NewBundleCollectionEntry(clustersPerPolicyBundle, fullStatusPredicate,
			bundleregistration.NewFullStateBundleRegistration(clustersPerPolicyTransportKey)),
		generic.NewBundleCollectionEntry(completeComplianceStatusBundle, fullStatusPredicate,
			bundleregistration.NewFullStateBundleRegistration(completeComplianceStatusTransportKey)),
		generic.NewBundleCollectionEntry(minimalComplianceStatusBundle, minimalStatusPredicate,
			bundleregistration.NewFullStateBundleRegistration(minimalComplianceStatusTransportKey)),
	}
}

func extractPolicyID(obj bundle.Object) (string, bool) {
	val, ok := obj.GetAnnotations()[datatypes.OriginOwnerReferenceAnnotation]
	return val, ok
}
