package localpolicies

import (
	"fmt"
	"time"

	policiesv1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/apps/v1"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
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
	// TODO: Move to datatype repo.
	localPlacementRules = "LocalPlacementRules"
)

func addLocalPlacementruleController(mgr ctrl.Manager, transport transport.Transport, syncInterval time.Duration,
	leafHubName string) error {
	createObjFunc := func() bundle.Object { return &policiesv1.PlacementRule{} }

	// Generating a new placement rule bundle.
	localPlacementruleTransportKey := fmt.Sprintf("%s.%s", leafHubName, localPlacementRules)
	localPolicySpecBundle := generic.NewBundleCollectionEntry(localPlacementruleTransportKey,
		bundle.NewGenericStatusBundle(leafHubName,
		helpers.GetBundleGenerationFromTransport(transport, localPlacementruleTransportKey, datatypes.SpecBundle)),
		func() bool { return true })

	bundleCollection := []*generic.BundleCollectionEntry{ localPolicySpecBundle }

	// TODO: check if predicate need to change.
	isLocalPlacementrulePred := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return !helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewGenericStatusSyncController(mgr, policiesStatusSyncLog, transport, policyCleanupFinalizer,
		bundleCollection, createObjFunc, syncInterval,
		isLocalPlacementrulePred); err != nil {
		return fmt.Errorf("failed to add controller to the manager - %w", err)
	}

	return nil
}