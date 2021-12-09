// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package policies

import (
	"errors"
	"fmt"
	"os"
	"strconv"

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
	policiesStatusSyncLog                             = "policies-status-sync"
	policyCleanupFinalizer                            = "hub-of-hubs.open-cluster-management.io/policy-cleanup"
	envVarComplianceStatusSentDeltasCountSwitchFactor = "COMPLIANCE_STATUS_DELTA_COUNT_SWITCH_FACTOR"
)

var (
	errFailedToCreateHybridSyncManager = errors.New("failed to create hybrid sync manager")
	errEnvVarNotFound                  = errors.New("environment variable not found")
	errEnvVarIllegalValue              = errors.New("environment variable illegal value")
)

// AddPoliciesStatusController adds policies status controller to the manager.
func AddPoliciesStatusController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	bundleCollection, err := createBundleCollection(transport, leafHubName, incarnation, hubOfHubsConfig)
	if err != nil {
		return fmt.Errorf("failed to add policies controller to the manager - %w", err)
	}

	hohNamespacePredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return meta.GetNamespace() == datatypes.HohSystemNamespace
	})
	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation)
	})

	createObjFunction := func() bundle.Object { return &policiesv1.Policy{} }

	// initialize policy status controller (contains multiple bundles)
	if err := generic.NewGenericStatusSyncController(mgr, policiesStatusSyncLog, transport, policyCleanupFinalizer,
		bundleCollection, createObjFunction, predicate.And(hohNamespacePredicate, ownerRefAnnotationPredicate),
		syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add policies controller to the manager - %w", err)
	}

	return nil
}

func createBundleCollection(transportObj transport.Transport, leafHubName string, incarnation uint64,
	hubOfHubsConfig *configv1.Config) ([]*generic.BundleCollectionEntry, error) {
	deltaSentCountSwitchFactor, err := readEnvVars()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize bundle collection - %w", err)
	}

	// clusters per policy (base bundle)
	clustersPerPolicyBundle := bundle.NewClustersPerPolicyBundle(leafHubName, incarnation, extractPolicyID)
	clustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, clustersPerPolicyBundle.GetID())

	// minimal compliance status bundle
	minimalComplianceStatusBundle := bundle.NewMinimalComplianceStatusBundle(leafHubName, incarnation)
	minimalComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, minimalComplianceStatusBundle.GetID())

	fullStatusPredicate := func() bool { return hubOfHubsConfig.Spec.AggregationLevel == configv1.Full }
	minimalStatusPredicate := func() bool { return hubOfHubsConfig.Spec.AggregationLevel == configv1.Minimal }

	// apply a hybrid sync manager on the (full aggregation) compliance bundles
	completeComplianceStatusBundleCollectionEntry, deltaComplianceStatusBundleCollectionEntry,
		err := getHybridComplianceBundleCollectionEntries(transportObj, leafHubName, incarnation, fullStatusPredicate,
		clustersPerPolicyBundle, deltaSentCountSwitchFactor)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize hybrid sync manager - %w", err)
	}

	// no need to send in the same cycle both clusters per policy and compliance. if CpP was sent, don't send compliance
	return []*generic.BundleCollectionEntry{ // multiple bundles for policy status
		generic.NewBundleCollectionEntry(clustersPerPolicyTransportKey, clustersPerPolicyBundle, fullStatusPredicate),
		completeComplianceStatusBundleCollectionEntry,
		deltaComplianceStatusBundleCollectionEntry,
		generic.NewBundleCollectionEntry(minimalComplianceStatusTransportKey, minimalComplianceStatusBundle,
			minimalStatusPredicate),
	}, nil
}

func readEnvVars() (int, error) {
	// delta bundle sent-count switch factor from env var
	deltaCountSwitchFactorString, found := os.LookupEnv(envVarComplianceStatusSentDeltasCountSwitchFactor)
	if !found {
		return 0, fmt.Errorf("%w: %s", errEnvVarNotFound, envVarComplianceStatusSentDeltasCountSwitchFactor)
	}

	deltaCountSwitchFactor, err := strconv.Atoi(deltaCountSwitchFactorString)
	if err != nil {
		return 0, fmt.Errorf("%w: %v - %s", err, errEnvVarIllegalValue,
			envVarComplianceStatusSentDeltasCountSwitchFactor)
	}

	return deltaCountSwitchFactor, nil
}

// getHybridComplianceBundleCollectionEntries creates a complete/delta compliance bundle collection entries and has
// them managed by a genericHybridSyncManager.
// The collection entries are returned (or nils with an error if any occurred).
func getHybridComplianceBundleCollectionEntries(transport transport.Transport, leafHubName string,
	incarnation uint64, fullStatusPredicate func() bool, clustersPerPolicyBundle bundle.Bundle,
	deltaCountSwitchFactor int) (*generic.BundleCollectionEntry, *generic.BundleCollectionEntry, error) {
	// complete compliance status bundle
	completeComplianceStatusBundle := bundle.NewCompleteComplianceStatusBundle(leafHubName, clustersPerPolicyBundle,
		incarnation, extractPolicyID)
	completeComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, completeComplianceStatusBundle.GetID())

	// delta compliance status bundle
	deltaComplianceStatusBundle := bundle.NewDeltaComplianceStatusBundle(leafHubName, completeComplianceStatusBundle,
		clustersPerPolicyBundle.(*bundle.ClustersPerPolicyBundle), incarnation, extractPolicyID)
	deltaComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, deltaComplianceStatusBundle.GetID())

	completeComplianceBundleCollectionEntry := generic.NewBundleCollectionEntry(completeComplianceStatusTransportKey,
		completeComplianceStatusBundle, fullStatusPredicate)
	deltaComplianceBundleCollectionEntry := generic.NewBundleCollectionEntry(deltaComplianceStatusTransportKey,
		deltaComplianceStatusBundle, fullStatusPredicate)

	if err := generic.NewHybridSyncManager(ctrl.Log.WithName("compliance-status hybrid sync manager"),
		transport, leafHubName, completeComplianceBundleCollectionEntry, deltaComplianceBundleCollectionEntry,
		deltaCountSwitchFactor); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", err, errFailedToCreateHybridSyncManager)
	}

	return completeComplianceBundleCollectionEntry, deltaComplianceBundleCollectionEntry, nil
}

func extractPolicyID(obj bundle.Object) (string, bool) {
	val, ok := obj.GetAnnotations()[datatypes.OriginOwnerReferenceAnnotation]
	return val, ok
}
