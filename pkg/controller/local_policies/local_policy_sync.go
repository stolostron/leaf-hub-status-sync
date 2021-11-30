package localpolicies

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
	localPoliciesStatusSyncLog  = "local-policies-status-sync"
	localPolicyCleanupFinalizer = "hub-of-hubs.open-cluster-management.io/local-policy-cleanup"
	rootPolicyLabel             = "policy.open-cluster-management.io/root-policy"
)

type bundleInfo struct {
	transportKey         string
	bundle               bundle.Bundle
	deliveryRegistration *transport.BundleDeliveryRegistration
}

// AddLocalPoliciesController this function adds a new local policies sync controller.
func AddLocalPoliciesController(mgr ctrl.Manager, transportObj transport.Transport, leafHubName string,
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	createObjFunc := func() bundle.Object { return &policiesv1.Policy{} }
	transportRetryChan := make(chan *transport.Message)

	bundleCollection := createBundleCollection(transportObj, leafHubName, incarnation, transportRetryChan,
		hubOfHubsConfig)

	localPolicyPredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return !helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation) &&
			!helpers.HasLabel(meta, rootPolicyLabel)
	})

	if err := generic.NewGenericStatusSyncController(mgr, localPoliciesStatusSyncLog, transportObj,
		localPolicyCleanupFinalizer, bundleCollection, createObjFunc, localPolicyPredicate, transportRetryChan,
		syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add local policies controller to the manager - %w", err)
	}

	return nil
}

func createBundleCollection(transport transport.Transport, leafHubName string, incarnation uint64,
	transportRetryChan chan *transport.Message, hubOfHubsConfig *configv1.Config) []*generic.BundleCollectionEntry {
	// clusters per policy (base bundle)
	localClustersPerPolicyBundleInfo := getLocalClustersPerPolicyBundleInfo(leafHubName, incarnation,
		transportRetryChan)

	// compliance status bundle
	localComplianceStatusBundleInfo := getLocalComplianceStatusBundleInfo(localClustersPerPolicyBundleInfo.bundle,
		leafHubName, incarnation, transportRetryChan)

	localPolicySpecBundleInfo := getLocalPolicySpecBundleInfo(leafHubName, incarnation, transportRetryChan)

	// register conditions
	addConditionsToBundles(localClustersPerPolicyBundleInfo.deliveryRegistration,
		localComplianceStatusBundleInfo.deliveryRegistration, localPolicySpecBundleInfo.deliveryRegistration,
		hubOfHubsConfig)

	// create bundle collection entries and register in transport
	bundleCollection := make([]*generic.BundleCollectionEntry, 0)
	for _, bundleInfo := range []*bundleInfo{
		localClustersPerPolicyBundleInfo, localComplianceStatusBundleInfo, localPolicySpecBundleInfo,
	} {
		bundleCollection = append(bundleCollection, generic.NewBundleCollectionEntry(bundleInfo.transportKey,
			bundleInfo.bundle, bundleInfo.deliveryRegistration))

		transport.Register(bundleInfo.transportKey, bundleInfo.deliveryRegistration)
	}
	// multiple bundles for local policies
	return bundleCollection
}

func addConditionsToBundles(localClustersPerPolicyRegistration, localCompleteComplianceRegistration,
	localPolicySpecDeliveryRegistration *transport.BundleDeliveryRegistration, hubOfHubsConfig *configv1.Config) {
	// check for full information
	localPolicyStatusPredicate := func(interface{}) bool {
		return hubOfHubsConfig.Spec.AggregationLevel == configv1.Full && hubOfHubsConfig.Spec.EnableLocalPolicies
	}

	helpers.AddConditionToDeliveryRegistrations([]*transport.BundleDeliveryRegistration{
		localClustersPerPolicyRegistration,
		localCompleteComplianceRegistration,
		localPolicySpecDeliveryRegistration,
	},
		transport.BeforeDeliveryAttempt, transport.ArgTypeNone, localPolicyStatusPredicate)
}

func getLocalClustersPerPolicyBundleInfo(leafHubName string, incarnation uint64,
	retryChan chan *transport.Message) *bundleInfo {
	localClustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.LocalClustersPerPolicyMsgKey)
	localClustersPerPolicyBundle := bundle.NewClustersPerPolicyBundle(leafHubName, incarnation, extractLocalPolicyID)
	localClustersPerPolicyDeliveryRegistration := transport.NewBundleDeliveryRegistration(retryChan, nil)

	return &bundleInfo{
		transportKey:         localClustersPerPolicyTransportKey,
		bundle:               localClustersPerPolicyBundle,
		deliveryRegistration: localClustersPerPolicyDeliveryRegistration,
	}
}

func getLocalComplianceStatusBundleInfo(localClustersPerPolicyBundle bundle.Bundle, leafHubName string,
	incarnation uint64, retryChan chan *transport.Message) *bundleInfo {
	localCompleteComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		datatypes.LocalPolicyCompleteComplianceMsgKey)
	localCompleteComplianceStatusBundle := bundle.NewCompleteComplianceStatusBundle(leafHubName,
		localClustersPerPolicyBundle, incarnation, extractLocalPolicyID)
	localCompleteComplianceStatusDeliveryRegistration := transport.NewBundleDeliveryRegistration(retryChan, nil)

	return &bundleInfo{
		transportKey:         localCompleteComplianceStatusTransportKey,
		bundle:               localCompleteComplianceStatusBundle,
		deliveryRegistration: localCompleteComplianceStatusDeliveryRegistration,
	}
}

func getLocalPolicySpecBundleInfo(leafHubName string, incarnation uint64,
	retryChan chan *transport.Message) *bundleInfo {
	localPolicySpecTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.LocalPolicySpecMsgKey)
	localPolicySpecBundle := bundle.NewGenericStatusBundle(leafHubName, incarnation, cleanPolicyFunc)
	localPolicySpecDeliveryRegistration := transport.NewBundleDeliveryRegistration(retryChan, nil)

	return &bundleInfo{
		transportKey:         localPolicySpecTransportKey,
		bundle:               localPolicySpecBundle,
		deliveryRegistration: localPolicySpecDeliveryRegistration,
	}
}

func cleanPolicyFunc(object bundle.Object) {
	policy, ok := object.(*policiesv1.Policy)
	if !ok {
		panic("Wrong instance passed to clean placement rule function, not appsv1.Policy")
	}

	policy.Status = policiesv1.PolicyStatus{}
}

func extractLocalPolicyID(obj bundle.Object) (string, bool) {
	return string(obj.GetUID()), true
}
