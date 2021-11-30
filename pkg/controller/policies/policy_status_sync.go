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

type bundleInfo struct {
	transportKey         string
	bundle               bundle.Bundle
	deliveryRegistration *transport.BundleDeliveryRegistration
}

// AddPoliciesStatusController adds policies status controller to the manager.
func AddPoliciesStatusController(mgr ctrl.Manager, transportObj transport.Transport, leafHubName string,
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunction := func() bundle.Object { return &policiesv1.Policy{} }
	transportRetryChan := make(chan *transport.Message)

	bundleCollection := createBundleCollection(transportObj, leafHubName, incarnation, transportRetryChan,
		hubOfHubsConfig)

	hohNamespacePredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return meta.GetNamespace() == datatypes.HohSystemNamespace
	})
	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation)
	})

	// initialize policy status controller (contains multiple bundles)
	if err := generic.NewGenericStatusSyncController(mgr, policiesStatusSyncLog, transportObj, policyCleanupFinalizer,
		bundleCollection, createObjFunction, predicate.And(hohNamespacePredicate, ownerRefAnnotationPredicate),
		transportRetryChan, syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add policies controller to the manager - %w", err)
	}

	return nil
}

func createBundleCollection(transportObj transport.Transport, leafHubName string, incarnation uint64,
	transportRetryChan chan *transport.Message,
	hubOfHubsConfig *configv1.Config) []*generic.BundleCollectionEntry {
	// clusters per policy (base bundle for non-minimal compliance status)
	clustersPerPolicyBundleInfo := getClustersPerPolicyBundleInfo(leafHubName, incarnation,
		transportRetryChan)

	// minimal compliance status bundle & delivery condition
	minComplianceStatusBundleInfo := getMinComplianceStatusBundleInfo(leafHubName, incarnation,
		transportRetryChan)

	// application of hybrid sync mode on compliance status
	completeComplianceStatusBundleInfo := getCompleteComplianceStatusBundleInfo(clustersPerPolicyBundleInfo.bundle,
		leafHubName, incarnation, transportRetryChan)

	// register conditions
	addConditionsToBundles(clustersPerPolicyBundleInfo.deliveryRegistration,
		completeComplianceStatusBundleInfo.deliveryRegistration, minComplianceStatusBundleInfo.deliveryRegistration,
		hubOfHubsConfig)

	// create bundle collection entries and register in transport
	bundleCollection := make([]*generic.BundleCollectionEntry, 0)
	for _, bundleInfo := range []*bundleInfo{
		clustersPerPolicyBundleInfo, minComplianceStatusBundleInfo,
		completeComplianceStatusBundleInfo,
	} {
		bundleCollection = append(bundleCollection, generic.NewBundleCollectionEntry(bundleInfo.transportKey,
			bundleInfo.bundle, bundleInfo.deliveryRegistration))

		transportObj.Register(bundleInfo.transportKey, bundleInfo.deliveryRegistration)
	}

	return bundleCollection
}

func addConditionsToBundles(clustersPerPolicyRegistration, completeComplianceRegistration,
	minComplianceRegistration *transport.BundleDeliveryRegistration, hubOfHubsConfig *configv1.Config) {
	// --- config before delivery conditions ---
	fullStatusPredicate := func(interface{}) bool { return hubOfHubsConfig.Spec.AggregationLevel == configv1.Full }
	minStatusPredicate := func(interface{}) bool { return hubOfHubsConfig.Spec.AggregationLevel == configv1.Minimal }

	// minimal
	minComplianceRegistration.AddCondition(transport.BeforeDeliveryAttempt, transport.ArgTypeNone, minStatusPredicate)
	// full
	helpers.AddConditionToDeliveryRegistrations([]*transport.BundleDeliveryRegistration{
		clustersPerPolicyRegistration,
		completeComplianceRegistration,
	},
		transport.BeforeDeliveryAttempt, transport.ArgTypeNone, fullStatusPredicate)
}

func getClustersPerPolicyBundleInfo(leafHubName string, incarnation uint64,
	retryChan chan *transport.Message) *bundleInfo {
	clustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.ClustersPerPolicyMsgKey)
	clustersPerPolicyBundle := bundle.NewClustersPerPolicyBundle(leafHubName, incarnation, 0, extractPolicyID)
	clustersPerPolicyDeliveryRegistration := transport.NewBundleDeliveryRegistration(0, retryChan, nil)

	return &bundleInfo{
		transportKey:         clustersPerPolicyTransportKey,
		bundle:               clustersPerPolicyBundle,
		deliveryRegistration: clustersPerPolicyDeliveryRegistration,
	}
}

func getMinComplianceStatusBundleInfo(leafHubName string, incarnation uint64,
	retryChan chan *transport.Message) *bundleInfo {
	minComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.MinimalPolicyComplianceMsgKey)
	minComplianceStatusBundle := bundle.NewMinimalComplianceStatusBundle(leafHubName, incarnation, 0)
	minComplianceStatusDeliveryRegistration := transport.NewBundleDeliveryRegistration(0, retryChan, nil)

	return &bundleInfo{
		transportKey:         minComplianceStatusTransportKey,
		bundle:               minComplianceStatusBundle,
		deliveryRegistration: minComplianceStatusDeliveryRegistration,
	}
}

func getCompleteComplianceStatusBundleInfo(clustersPerPolicyBundle bundle.Bundle,
	leafHubName string, incarnation uint64, retryChan chan *transport.Message) *bundleInfo {
	completeComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		datatypes.PolicyCompleteComplianceMsgKey)
	completeComplianceStatusBundle := bundle.NewCompleteComplianceStatusBundle(leafHubName, incarnation, 0,
		clustersPerPolicyBundle, extractPolicyID)
	completeComplianceStatusDeliveryRegistration := transport.NewBundleDeliveryRegistration(0, retryChan, nil)

	return &bundleInfo{
		transportKey:         completeComplianceStatusTransportKey,
		bundle:               completeComplianceStatusBundle,
		deliveryRegistration: completeComplianceStatusDeliveryRegistration,
	}
}

func extractPolicyID(obj bundle.Object) (string, bool) {
	val, ok := obj.GetAnnotations()[datatypes.OriginOwnerReferenceAnnotation]
	return val, ok
}
