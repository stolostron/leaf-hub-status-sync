package policies

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	v1 "github.com/open-cluster-management/governance-policy-propagator/pkg/apis/policy/v1"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/generic"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/predicate"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlpredicate "sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"
)

func newPolicyFinalizerCleanerController(mgr ctrl.Manager, logName string, finalizerName string) error {
	policyFinalizerCleanerCtrl := &policyFinalizerCleanerController{
		client:        mgr.GetClient(),
		log:           ctrl.Log.WithName(logName),
		finalizerName: finalizerName,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Policy{}).
		WithEventFilter(ctrlpredicate.And(generic.Predicate,
			&predicate.PolicyFinalizerCleanerPredicate{FinalizerName: finalizerName})).
		Complete(policyFinalizerCleanerCtrl)
}

type policyFinalizerCleanerController struct {
	client        client.Client
	log           logr.Logger
	finalizerName string
}

func (c *policyFinalizerCleanerController) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	ctx := context.Background()
	object := &v1.Policy{}
	err := c.client.Get(ctx, request.NamespacedName, object)
	if apierrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}
	if err != nil {
		reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
		return ctrl.Result{Requeue: true, RequeueAfter: generic.RequeuePeriodSeconds * time.Second}, err
	}
	if err = c.removeFinalizer(ctx, object, c.log); err != nil {
		reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
		return ctrl.Result{Requeue: true, RequeueAfter: generic.RequeuePeriodSeconds * time.Second}, err
	}
	reqLogger.Info("Reconciliation complete.")
	return ctrl.Result{}, err
}

func (c *policyFinalizerCleanerController) removeFinalizer(ctx context.Context, policy *v1.Policy, log logr.Logger) error {
	if !helpers.ContainsString(policy.GetFinalizers(), c.finalizerName) {
		return nil // if finalizer is not there, do nothing
	}

	log.Info("removing finalizer")
	controllerutil.RemoveFinalizer(policy, c.finalizerName)
	if err := c.client.Update(ctx, policy); err != nil {
		return fmt.Errorf("failed to remove finalizer %s, requeue in order to retry", c.finalizerName)
	}
	return nil
}
