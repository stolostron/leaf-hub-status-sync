package localpolicies

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
	policiesStatusSyncLog  = "local-policies-status-sync"
	policyCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/local-policy-cleanup"
	// may need to move.
	rootReferenceLabel = "policy.open-cluster-management.io/root-local-policy"
)

// AddLocalPoliciesController this function adds a new local policies sync controller.
func AddLocalPoliciesController(mgr ctrl.Manager, transport transport.Transport, syncInterval time.Duration,
	leafHubName string, hubOfHubsConfig *configv1.Config) error {
	createObjFunc := func() bundle.Object { return &policiesv1.Policy{} }

	extractLocalPolicyIDFunc := func(obj bundle.Object) (string, bool) { return string(obj.GetUID()), true }

	// clusters per policy (base bundle)
	localClustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.LocalClustersPerPolicyMsgKey)
	localClustersPerPolicyBundle := bundle.NewClustersPerPolicyBundle(leafHubName,
		helpers.GetGenerationFromTransport(transport, localClustersPerPolicyTransportKey, datatypes.StatusBundle),
		extractLocalPolicyIDFunc)

	// compliance status bundle
	localCompleteComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		datatypes.PolicyCompleteComplianceMsgKey)
	localCompleteComplianceStatusBundle := bundle.NewCompleteComplianceStatusBundle(leafHubName,
		localClustersPerPolicyBundle, helpers.GetGenerationFromTransport(transport,
			localCompleteComplianceStatusTransportKey, datatypes.StatusBundle), extractLocalPolicyIDFunc)

	localSpecPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.LocalPolicySpecMsgKey)
	localPolicySpecBundle := generic.NewBundleCollectionEntry(localSpecPerPolicyTransportKey,
		bundle.NewGenericStatusBundle(leafHubName,
			helpers.GetGenerationFromTransport(transport, localSpecPerPolicyTransportKey, datatypes.StatusBundle),
			cleanPolicyFunc),
		func() bool { return hubOfHubsConfig.Spec.EnableLocalPolicies })

	// check for full information
	fullStatusPredicate := func() bool {
		return hubOfHubsConfig.Spec.AggregationLevel == configv1.Full && hubOfHubsConfig.Spec.EnableLocalPolicies
	}

	bundleCollection := []*generic.BundleCollectionEntry{ // multiple bundles for policy status
		generic.NewBundleCollectionEntry(localClustersPerPolicyTransportKey,
			localClustersPerPolicyBundle, fullStatusPredicate),
		generic.NewBundleCollectionEntry(localCompleteComplianceStatusTransportKey,
			localCompleteComplianceStatusBundle, fullStatusPredicate), localPolicySpecBundle,
	}

	localPolicyPredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return !helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation) &&
			!helpers.HasLabel(meta, rootReferenceLabel)
	})

	if err := generic.NewGenericStatusSyncController(mgr, policiesStatusSyncLog, transport, policyCleanupFinalizer,
		bundleCollection, createObjFunc, syncInterval,
		localPolicyPredicate); err != nil {
		return fmt.Errorf("failed to add local policies controller to the manager - %w", err)
	}

	return nil
}

func cleanPolicyFunc(object bundle.Object) {
	policy, ok := object.(*policiesv1.Policy)
	if !ok {
		panic("Wrong instance passed to clean placement rule function, not appsv1.Policy")
	}

	policy.Status = policiesv1.PolicyStatus{}
}
