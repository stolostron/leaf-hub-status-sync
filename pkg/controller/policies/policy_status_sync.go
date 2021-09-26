// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package policies

import (
	"errors"
	"fmt"
	"os"
	"strconv"
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
	envVarHybridModeDeltasCountModeSwitchFactor = "COMPLIANCE_HYBRID_MODE_DELTA_COUNT_SWITCH"
	envVarHybridModeFailCountModeSwitchFactor   = "COMPLIANCE_HYBRID_MODE_FAIL_COUNT_SWITCH"
	policiesStatusSyncLog                       = "policies-status-sync"
	policyCleanupFinalizer                      = "hub-of-hubs.open-cluster-management.io/policy-cleanup"
	internalChanBufferSize                      = 42
)

var (
	errEnvVarNotFound     = errors.New("not found environment variable")
	errEnvVarIllegalValue = errors.New("environment variable illegal value")
)

type bundleInfo struct {
	transportKey         string
	bundle               bundle.Bundle
	deliveryRegistration *transport.BundleDeliveryRegistration
}

// AddPoliciesStatusController adds policies status controller to the manager.
func AddPoliciesStatusController(mgr ctrl.Manager, transportObj transport.Transport, syncInterval time.Duration,
	leafHubName string, incarnation uint64, hubOfHubsConfig *configv1.Config) error {
	transportRetryChan := make(chan *transport.Message, internalChanBufferSize)

	// clusters per policy (base bundle for non-minimal compliance status)
	clustersPerPolicyBundleInfo := getClustersPerPolicyBundleInfo(leafHubName, incarnation,
		transportRetryChan)

	// minimal compliance status bundle & delivery condition
	minComplianceStatusBundleInfo := getMinComplianceStatusBundleInfo(leafHubName, incarnation,
		transportRetryChan)

	// application of hybrid sync mode on compliance status
	completeComplianceStatusBundleInfo, deltaComplianceStatusBundleInfo,
		err := initHybridComplianceStatusManager(mgr, leafHubName, incarnation,
		clustersPerPolicyBundleInfo.bundle, transportRetryChan)
	if err != nil {
		return fmt.Errorf("failed to add hybrid status controller to the manager - %w", err)
	}

	// register conditions
	addConditionsToBundles(clustersPerPolicyBundleInfo.deliveryRegistration,
		completeComplianceStatusBundleInfo.deliveryRegistration, deltaComplianceStatusBundleInfo.deliveryRegistration,
		minComplianceStatusBundleInfo.deliveryRegistration, hubOfHubsConfig)

	// create bundle collection entries and register in transport
	bundleCollection := make([]*generic.BundleCollectionEntry, 0)
	for _, bundleInfo := range []*bundleInfo{
		clustersPerPolicyBundleInfo, minComplianceStatusBundleInfo,
		completeComplianceStatusBundleInfo, deltaComplianceStatusBundleInfo,
	} {
		bundleCollection = append(bundleCollection, generic.NewBundleCollectionEntry(bundleInfo.transportKey,
			bundleInfo.bundle, bundleInfo.deliveryRegistration))

		transportObj.Register(bundleInfo.transportKey, bundleInfo.deliveryRegistration)
	}

	hohNamespacePredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return meta.GetNamespace() == datatypes.HohSystemNamespace
	})
	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation)
	})

	// initialize policy status controller (contains multiple bundles)
	createObjFunction := func() bundle.Object { return &policiesv1.Policy{} }
	if err := generic.NewGenericStatusSyncController(mgr, policiesStatusSyncLog, incarnation, transportObj,
		policyCleanupFinalizer, bundleCollection, createObjFunction, syncInterval, transportRetryChan,
		predicate.And(hohNamespacePredicate, ownerRefAnnotationPredicate)); err != nil {
		return fmt.Errorf("failed to add controller to the manager - %w", err)
	}

	return nil
}

func addConditionsToBundles(clustersPerPolicyRegistration, completeComplianceRegistration, deltaComplianceRegistration,
	minComplianceRegistration *transport.BundleDeliveryRegistration, hubOfHubsConfig *configv1.Config) {
	// --- config before delivery conditions ---
	fullStatusPredicate := func(_ interface{}) bool { return hubOfHubsConfig.Spec.AggregationLevel == configv1.Full }
	minStatusPredicate := func(_ interface{}) bool { return hubOfHubsConfig.Spec.AggregationLevel == configv1.Minimal }

	// minimal
	minComplianceRegistration.AddCondition(transport.BeforeDeliveryAttempt, transport.ArgTypeNone, minStatusPredicate)
	// full
	helpers.AddConditionToDeliveryRegistrations([]*transport.BundleDeliveryRegistration{
		clustersPerPolicyRegistration,
		completeComplianceRegistration,
		deltaComplianceRegistration,
	},
		transport.BeforeDeliveryAttempt, transport.ArgTypeNone, fullStatusPredicate)

	// --- generations before delivery conditions ---
	for _, registration := range []*transport.BundleDeliveryRegistration{
		clustersPerPolicyRegistration,
		completeComplianceRegistration,
		deltaComplianceRegistration,
		minComplianceRegistration,
	} {
		reg := registration
		registration.AddCondition(transport.BeforeDeliveryAttempt, transport.ArgTypeBundleGeneration,
			func(generation interface{}) bool {
				return generation.(uint64) > reg.GetLastSentGeneration()
			})
	}

	// --- before delivery retry conditions ---
	for _, registration := range []*transport.BundleDeliveryRegistration{
		clustersPerPolicyRegistration,
		completeComplianceRegistration,
		minComplianceRegistration,
	} {
		reg := registration
		registration.AddCondition(transport.BeforeDeliveryRetry, transport.ArgTypeBundleGeneration,
			func(generation interface{}) bool {
				return generation.(uint64) == reg.GetLastSentGeneration()
			})
	}
}

func getClustersPerPolicyBundleInfo(leafHubName string, incarnation uint64,
	retryChan chan *transport.Message) *bundleInfo {
	clustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.ClustersPerPolicyMsgKey)
	clustersPerPolicyBundle := bundle.NewClustersPerPolicyBundle(leafHubName, incarnation, 0)
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
	leafHubName string, incarnation uint64, existingPoliciesMap map[string]struct{}, retryChan chan *transport.Message,
	failChan chan *error) *bundleInfo {
	completeComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		datatypes.PolicyCompleteComplianceMsgKey)
	completeComplianceStatusBundle := bundle.NewCompleteComplianceStatusBundle(leafHubName, incarnation, 0,
		clustersPerPolicyBundle, existingPoliciesMap)
	completeComplianceStatusDeliveryRegistration := transport.NewBundleDeliveryRegistration(0, retryChan, failChan)

	return &bundleInfo{
		transportKey:         completeComplianceStatusTransportKey,
		bundle:               completeComplianceStatusBundle,
		deliveryRegistration: completeComplianceStatusDeliveryRegistration,
	}
}

func getDeltaComplianceStatusBundleInfo(clustersPerPolicyBundle bundle.Bundle,
	completeComplianceBundle bundle.HybridBundle, leafHubName string, incarnation uint64,
	existingPoliciesMap map[string]struct{}, retryChan chan *transport.Message, failChan chan *error) *bundleInfo {
	deltaComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		datatypes.PolicyDeltaComplianceMsgKey)
	deltaComplianceStatusBundle := bundle.NewDeltaComplianceStatusBundle(leafHubName, incarnation, 0,
		clustersPerPolicyBundle, completeComplianceBundle, existingPoliciesMap)
	deltaComplianceStatusDeliveryRegistration := transport.NewBundleDeliveryRegistration(0, retryChan, failChan)

	return &bundleInfo{
		transportKey:         deltaComplianceStatusTransportKey,
		bundle:               deltaComplianceStatusBundle,
		deliveryRegistration: deltaComplianceStatusDeliveryRegistration,
	}
}

// initHybridComplianceStatusManager starts a new instance of genericHybridStatusManager and returns:
// completeComplianceStatusBundleInfo
// deltaComplianceStatusBundleInfo
// error.
func initHybridComplianceStatusManager(mgr ctrl.Manager, leafHubName string,
	incarnation uint64, clustersPerPolicyBundle bundle.Bundle, retryChan chan *transport.Message) (*bundleInfo,
	*bundleInfo, error) {
	// mode management env vars for hybrid status controller
	sentDeltaCountSwitchFactor, failCountSwitchFactor, err := getHybridModeEnvVars()
	if err != nil {
		return nil, nil, fmt.Errorf("%w", err)
	}

	// policies map to serve as policies cache for delta bundles
	existingPoliciesMap := make(map[string]struct{})
	// delivery fail channel
	failChan := make(chan *error, internalChanBufferSize)

	// complete compliance status
	cComplianceStatusBundleInfo := getCompleteComplianceStatusBundleInfo(clustersPerPolicyBundle,
		leafHubName, incarnation, existingPoliciesMap, retryChan, failChan)
	cComplianceStatusBundle, _ := cComplianceStatusBundleInfo.bundle.(bundle.HybridBundle)

	// delta compliance status bundle
	dComplianceStatusBundleInfo := getDeltaComplianceStatusBundleInfo(clustersPerPolicyBundle,
		cComplianceStatusBundle, leafHubName, incarnation, existingPoliciesMap, retryChan, failChan)
	dComplianceStatusBundle, _ := dComplianceStatusBundleInfo.bundle.(bundle.DeltaStateBundle)

	// hybrid compliance status manager
	hybridComplianceStatusManager := generic.NewGenericHybridSyncController(sentDeltaCountSwitchFactor,
		failCountSwitchFactor, cComplianceStatusBundle, dComplianceStatusBundle,
		cComplianceStatusBundleInfo.deliveryRegistration, dComplianceStatusBundleInfo.deliveryRegistration, failChan)

	if err := mgr.Add(hybridComplianceStatusManager); err != nil {
		return nil, nil, fmt.Errorf("%w", err)
	}

	return cComplianceStatusBundleInfo, dComplianceStatusBundleInfo, nil
}

func getHybridModeEnvVars() (int, int, error) {
	sentDeltaCountSwitchFactorString, found := os.LookupEnv(envVarHybridModeDeltasCountModeSwitchFactor)
	if !found {
		return 0, 0, fmt.Errorf("%w : %s", errEnvVarNotFound, envVarHybridModeDeltasCountModeSwitchFactor)
	}

	failCountSwitchFactorString, found := os.LookupEnv(envVarHybridModeFailCountModeSwitchFactor)
	if !found {
		return 0, 0, fmt.Errorf("%w : %s", errEnvVarNotFound, envVarHybridModeFailCountModeSwitchFactor)
	}

	sentDeltaCountSwitchFactor, legal := strconv.Atoi(sentDeltaCountSwitchFactorString)
	if legal != nil {
		return 0, 0, fmt.Errorf("%w : %s", errEnvVarIllegalValue, envVarHybridModeDeltasCountModeSwitchFactor)
	}

	failCountSwitchFactor, legal := strconv.Atoi(failCountSwitchFactorString)
	if legal != nil {
		return 0, 0, fmt.Errorf("%w : %s", errEnvVarIllegalValue, envVarHybridModeFailCountModeSwitchFactor)
	}

	return sentDeltaCountSwitchFactor, failCountSwitchFactor, nil
}
