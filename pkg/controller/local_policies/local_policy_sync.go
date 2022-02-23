package localpolicies

import (
	"fmt"

	policiesv1 "github.com/open-cluster-management/governance-policy-propagator/api/v1"
	datatypes "github.com/stolostron/hub-of-hubs-data-types"
	configv1 "github.com/stolostron/hub-of-hubs-data-types/apis/config/v1"
	"github.com/stolostron/leaf-hub-status-sync/pkg/bundle"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/stolostron/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/stolostron/leaf-hub-status-sync/pkg/helpers"
	"github.com/stolostron/leaf-hub-status-sync/pkg/transport"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	localPoliciesStatusSyncLog  = "local-policies-status-sync"
	localPolicyCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/local-policy-cleanup"
	rootPolicyLabel             = "policy.open-cluster-management.io/root-policy"
)

// AddLocalPoliciesController this function adds a new local policies sync controller.
func AddLocalPoliciesController(mgr ctrl.Manager, transport transport.Transport, leafHubName string,
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunc := func() bundle.Object { return &policiesv1.Policy{} }
	bundleCollection := createBundleCollection(leafHubName, incarnation, hubOfHubsConfig)

	localPolicyPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return !helpers.HasAnnotation(object, datatypes.OriginOwnerReferenceAnnotation) &&
			!helpers.HasLabel(object, rootPolicyLabel)
	})

	if err := generic.NewGenericStatusSyncController(mgr, localPoliciesStatusSyncLog, transport,
		localPolicyCleanupFinalizer, nil, bundleCollection, createObjFunc, localPolicyPredicate,
		syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add local policies controller to the manager - %w", err)
	}

	return nil
}

func createBundleCollection(leafHubName string, incarnation uint64,
	hubOfHubsConfig *configv1.Config) []*generic.BundleCollectionEntry {
	extractLocalPolicyIDFunc := func(obj bundle.Object) (string, bool) { return string(obj.GetUID()), true }

	// clusters per policy (base bundle)
	localClustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		datatypes.LocalClustersPerPolicyMsgKey)
	localClustersPerPolicyBundle := bundle.NewClustersPerPolicyBundle(leafHubName, incarnation,
		extractLocalPolicyIDFunc)

	// compliance status bundle
	localCompleteComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		datatypes.LocalPolicyCompleteComplianceMsgKey)
	localCompleteComplianceStatusBundle := bundle.NewCompleteComplianceStatusBundle(leafHubName,
		localClustersPerPolicyBundle, incarnation, extractLocalPolicyIDFunc)

	localPolicySpecTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.LocalPolicySpecMsgKey)
	localPolicySpecBundle := bundle.NewGenericStatusBundle(leafHubName, incarnation, cleanPolicyFunc)

	// check for full information
	localPolicyStatusPredicate := func() bool {
		return hubOfHubsConfig.Spec.AggregationLevel == configv1.Full && hubOfHubsConfig.Spec.EnableLocalPolicies
	}
	// multiple bundles for local policies
	return []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(localClustersPerPolicyTransportKey,
			localClustersPerPolicyBundle, localPolicyStatusPredicate),
		generic.NewBundleCollectionEntry(localCompleteComplianceStatusTransportKey,
			localCompleteComplianceStatusBundle, localPolicyStatusPredicate),
		generic.NewBundleCollectionEntry(localPolicySpecTransportKey, localPolicySpecBundle,
			func() bool { return hubOfHubsConfig.Spec.EnableLocalPolicies }),
	}
}

func cleanPolicyFunc(object bundle.Object) {
	policy, ok := object.(*policiesv1.Policy)
	if !ok {
		panic("Wrong instance passed to clean placement rule function, not appsv1.Policy")
	}

	policy.Status = policiesv1.PolicyStatus{}
}
