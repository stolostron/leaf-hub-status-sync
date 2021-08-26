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
	policiesStatusSyncLog  = "policies-status-sync"
	policyCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/policy-cleanup"
	rootReferenceLabel     = "policy.open-cluster-management.io/root-policy"
)

// AddLocalPoliciesController this function adds a new local policies sync controller.
func AddLocalPoliciesController(mgr ctrl.Manager, transport transport.Transport, syncInterval time.Duration,
	leafHubName string, hubOfHubsConfig *configv1.Config) error {
	createObjFunc := func() bundle.Object { return &policiesv1.Policy{} }

	// clusters per policy (base bundle)
	localClustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.LocalClustersPerPolicyMsgKey)
	localClustersPerPolicyBundle := bundle.NewClustersPerPolicyBundle(leafHubName,
		helpers.GetBundleGenerationFromTransport(transport, localClustersPerPolicyTransportKey, datatypes.StatusBundle))

	// compliance status bundle
	localComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.LocalPolicyComplianceMsgKey)
	localComplianceStatusBundle := bundle.NewComplianceStatusBundle(leafHubName, localClustersPerPolicyBundle,
		helpers.GetBundleGenerationFromTransport(transport, localComplianceStatusTransportKey, datatypes.StatusBundle))

	// spec per policy bundle
	cleanFunc :=
		func(object bundle.Object) (bundle.Object, bool) {
			policy, ok := object.(*policiesv1.Policy)
			if !ok {
				return nil, ok
			}

			policy.Status = policiesv1.PolicyStatus{}

			return policy, true
		}

	localSpecPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.LocalSpecPerPolicyMsgKey)
	localPolicySpecBundle := generic.NewBundleCollectionEntry(localSpecPerPolicyTransportKey,
		bundle.NewGenericStatusBundle(leafHubName,
			helpers.GetBundleGenerationFromTransport(transport, localSpecPerPolicyTransportKey, datatypes.StatusBundle),
			cleanFunc),
		func() bool { return true })

	// check for full information
	fullStatusPredicate := func() bool { return true }

	bundleCollection := []*generic.BundleCollectionEntry{ // multiple bundles for policy status
		generic.NewBundleCollectionEntry(localClustersPerPolicyTransportKey,
			localClustersPerPolicyBundle, fullStatusPredicate),
		generic.NewBundleCollectionEntry(localComplianceStatusTransportKey, localComplianceStatusBundle, fullStatusPredicate),
		localPolicySpecBundle,
	}

	isLocalPolicyPredic := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return !helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation) &&
			!helpers.HasLabel(meta, rootReferenceLabel)
	})

	if err := generic.NewGenericStatusSyncController(mgr, policiesStatusSyncLog, transport, policyCleanupFinalizer,
		bundleCollection, createObjFunc, syncInterval,
		isLocalPolicyPredic); err != nil {
		return fmt.Errorf("local policy failed to add controller to the manager - %w", err)
	}

	return nil
}
