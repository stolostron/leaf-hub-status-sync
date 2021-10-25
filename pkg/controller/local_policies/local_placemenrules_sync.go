package localpolicies

import (
	"fmt"
	"time"

	placementrulev1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/apps/v1"
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

// AddLocalPlacementruleController This function adds a new local placement rule controller.
func AddLocalPlacementruleController(mgr ctrl.Manager, transport transport.Transport, syncInterval time.Duration,
	leafHubName string, hubOfHubsConfig *configv1.Config) error {
	createObjFunc := func() bundle.Object { return &placementrulev1.PlacementRule{} }

	// Generating a new placement rule bundle.
	cleanFunc :=
		func(object bundle.Object) (bundle.Object, bool) {
			placement, ok := object.(*placementrulev1.PlacementRule)
			if !ok {
				return nil, ok
			}

			placement.Status = placementrulev1.PlacementRuleStatus{}

			return placement, true
		}
	localPlacementruleTransportKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.LocalPlacementRulesMsgKey)
	localPlacementRuleBundle := generic.NewBundleCollectionEntry(localPlacementruleTransportKey,
		bundle.NewGenericStatusBundle(leafHubName,
			helpers.GetBundleGenerationFromTransport(transport, localPlacementruleTransportKey, datatypes.StatusBundle),
			cleanFunc),
		func() bool { // bundle predicate
			return hubOfHubsConfig.Spec.EnableLocalPolicies
		})

	bundleCollection := []*generic.BundleCollectionEntry{localPlacementRuleBundle}

	isLocalPlacementrulePred := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return !helpers.HasAnnotation(meta, datatypes.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewGenericStatusSyncController(mgr, policiesStatusSyncLog, transport, policyCleanupFinalizer,
		bundleCollection, createObjFunc, syncInterval,
		isLocalPlacementrulePred); err != nil {
		return fmt.Errorf("placement rule failed to add controller to the manager - %w", err)
	}

	return nil
}
