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
	// TODO: Move to datatype repo.
	localClustersPerPolicy = "LocalClustersPerPolicy"
	// TODO: Move to datatype repo.
	localPolicyComplianceKey = "LocalPolicyCompliance"
	// TODO: Move to datatype repo.
	localSpecPerPolicy = "localSpecPerPolicy"
)

func AddLocalPoliciesController(mgr ctrl.Manager, transport transport.Transport, syncInterval time.Duration,
	leafHubName string, hubOfHubsConfig *configv1.Config) error {
	createObjFunc := func() bundle.Object { return &policiesv1.Policy{} }

	// clusters per policy (base bundle)
	localClustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, localClustersPerPolicy)
	localClustersPerPolicyBundle := bundle.NewClustersPerPolicyBundle(leafHubName,
		helpers.GetBundleGenerationFromTransport(transport, localClustersPerPolicyTransportKey, datatypes.StatusBundle))

	// compliance status bundle
	localComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName, localPolicyComplianceKey)
	localComplianceStatusBundle := bundle.NewComplianceStatusBundle(leafHubName, localClustersPerPolicyBundle,
		helpers.GetBundleGenerationFromTransport(transport, localComplianceStatusTransportKey, datatypes.StatusBundle))

	// spec per policy bundle
	localSpecPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, localSpecPerPolicy)
	localPolicySpecBundle := generic.NewBundleCollectionEntry(localSpecPerPolicyTransportKey,
		bundle.NewGenericStatusBundle(leafHubName,
			helpers.GetBundleGenerationFromTransport(transport, localSpecPerPolicyTransportKey, datatypes.SpecBundle)),
		func() bool { return true })

	// check for full information
	fullStatusPredicate := func() bool { return hubOfHubsConfig.Spec.AggregationLevel == configv1.Full }

	bundleCollection := []*generic.BundleCollectionEntry{ // multiple bundles for policy status
		generic.NewBundleCollectionEntry(localClustersPerPolicyTransportKey,
			localClustersPerPolicyBundle, fullStatusPredicate),
		generic.NewBundleCollectionEntry(localComplianceStatusTransportKey, localComplianceStatusBundle, fullStatusPredicate),
		localPolicySpecBundle,
	}

	isLocalPolicyPredic := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return !helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewGenericStatusSyncController(mgr, policiesStatusSyncLog, transport, policyCleanupFinalizer,
		bundleCollection, createObjFunc, syncInterval,
		isLocalPolicyPredic); err != nil {
		return fmt.Errorf("failed to add controller to the manager - %w", err)
	}

	return nil
}
