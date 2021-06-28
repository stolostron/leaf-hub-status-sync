package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-logr/logr"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/predicate"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlpredicate "sigs.k8s.io/controller-runtime/pkg/predicate"
	"strconv"
	"sync"
	"time"
)

const (
	RequeuePeriodSeconds = 5
)

type CreateObjectFunction func() bundle.Object

var Predicate = &predicate.GenericPredicate{}

func NewGenericStatusSyncController(mgr ctrl.Manager, logName string, transport transport.Transport,
	finalizerName string, orderedBundleCollection []*BundleCollectionEntry, createObjFunc CreateObjectFunction,
	syncInterval time.Duration, genericPredicateFilter bool, additionalPredicate ctrlpredicate.Predicate) error {
	statusSyncCtrl := &genericStatusSyncController{
		client:                  mgr.GetClient(),
		log:                     ctrl.Log.WithName(logName),
		transport:               transport,
		orderedBundleCollection: orderedBundleCollection,
		finalizerName:           finalizerName,
		createObjFunc:           createObjFunc,
		periodicSyncInterval:    syncInterval,
		genericPredicateFilter:  genericPredicateFilter,
	}
	statusSyncCtrl.init()

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).For(createObjFunc())
	if predicateToSet := getPredicate(genericPredicateFilter, additionalPredicate); predicateToSet != nil {
		controllerBuilder = controllerBuilder.WithEventFilter(predicateToSet)
	}
	return controllerBuilder.Complete(statusSyncCtrl)
}

func getPredicate(genericPredicateFilter bool, additionalPredicate ctrlpredicate.Predicate) ctrlpredicate.Predicate {
	if genericPredicateFilter { // generic predicate is true
		if additionalPredicate != nil {
			return ctrlpredicate.And(Predicate, additionalPredicate) // both generic and additional predicates
		}
		return Predicate
	}
	// if we got here, genericPredicateFilter is false
	if additionalPredicate != nil {
		return additionalPredicate
	}
	return nil
}

type genericStatusSyncController struct {
	client                  client.Client
	log                     logr.Logger
	transport               transport.Transport
	orderedBundleCollection []*BundleCollectionEntry
	finalizerName           string
	createObjFunc           CreateObjectFunction
	periodicSyncInterval    time.Duration
	startOnce               sync.Once
	genericPredicateFilter  bool
}

func (c *genericStatusSyncController) init() {
	c.startOnce.Do(func() {
		go c.periodicSync()
	})
}

func (c *genericStatusSyncController) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	ctx := context.Background()
	object := c.createObjFunc()
	err := c.client.Get(ctx, request.NamespacedName, object)
	if apierrors.IsNotFound(err) {
		// the instance was deleted and it had no finalizer on it.
		// this means either LH removed the finalizer so it was already deleted from bundle, or
		// LH didn't update HoH about this object ever.
		// either way, no need to do anything in this state.
		return ctrl.Result{}, nil
	}
	if err != nil {
		reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
		return ctrl.Result{Requeue: true, RequeueAfter: RequeuePeriodSeconds * time.Second}, err
	}
	if c.isObjectBeingDeleted(object) {
		if err = c.deleteObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
			return ctrl.Result{Requeue: true, RequeueAfter: RequeuePeriodSeconds * time.Second}, err
		}
	} else { // otherwise, the object was not deleted and no error occurred
		if err = c.updateObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
			return ctrl.Result{Requeue: true, RequeueAfter: RequeuePeriodSeconds * time.Second}, err
		}
	}
	reqLogger.Info("Reconciliation complete.")
	return ctrl.Result{}, err
}

func (c *genericStatusSyncController) isObjectBeingDeleted(object bundle.Object) bool {
	return !object.GetDeletionTimestamp().IsZero()
}

func (c *genericStatusSyncController) updateObjectAndFinalizer(ctx context.Context, object bundle.Object,
	log logr.Logger) error {
	if err := c.addFinalizer(ctx, object, log); err != nil {
		return err
	}
	cleanObject(object)
	for _, entry := range c.orderedBundleCollection {
		entry.bundle.UpdateObject(object) // update in each bundle from the collection according to their order
	}

	return nil
}

func (c *genericStatusSyncController) addFinalizer(ctx context.Context, object bundle.Object, log logr.Logger) error {
	if helpers.ContainsString(object.GetFinalizers(), c.finalizerName) {
		return nil
	}

	log.Info("adding finalizer")
	controllerutil.AddFinalizer(object, c.finalizerName)
	if err := c.client.Update(ctx, object); err != nil {
		return fmt.Errorf("failed to add finalizer %s, requeue in order to retry", c.finalizerName)
	}
	return nil
}
func (c *genericStatusSyncController) deleteObjectAndFinalizer(ctx context.Context, object bundle.Object,
	log logr.Logger) error {
	for _, entry := range c.orderedBundleCollection {
		entry.bundle.DeleteObject(object) // delete from all bundles
	}
	return c.removeFinalizer(ctx, object, log)
}

func (c *genericStatusSyncController) removeFinalizer(ctx context.Context, object bundle.Object, log logr.Logger) error {
	if !helpers.ContainsString(object.GetFinalizers(), c.finalizerName) {
		return nil // if finalizer is not there, do nothing
	}

	log.Info("removing finalizer")
	controllerutil.RemoveFinalizer(object, c.finalizerName)
	if err := c.client.Update(ctx, object); err != nil {
		return fmt.Errorf("failed to remove finalizer %s, requeue in order to retry", c.finalizerName)
	}
	return nil
}

func (c *genericStatusSyncController) periodicSync() {
	ticker := time.NewTicker(c.periodicSyncInterval)
	for {
		select {
		case <-ticker.C:
			for _, entry := range c.orderedBundleCollection {
				bundleGeneration := entry.bundle.GetBundleGeneration()
				if bundleGeneration > entry.lastSentBundleGeneration { // send to transport only if bundle has changed
					c.syncToTransport(entry.transportBundleKey, datatypes.StatusBundle,
						strconv.FormatUint(bundleGeneration, 10), entry.bundle)
					entry.lastSentBundleGeneration = bundleGeneration
				}
			}
		}
	}
}

func (c *genericStatusSyncController) syncToTransport(id string, objType string, generation string,
	payload bundle.Bundle) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		c.log.Info(fmt.Sprintf("failed to sync object from type %s with id %s- %s", objType, id, err))
		return
	}
	c.transport.SendAsync(id, objType, generation, payloadBytes)
}

func cleanObject(object bundle.Object) {
	object.SetManagedFields(nil)
	object.SetFinalizers(nil)
	object.SetGeneration(0)
	object.SetOwnerReferences(nil)
	object.SetSelfLink("")
	object.SetClusterName("")
}
