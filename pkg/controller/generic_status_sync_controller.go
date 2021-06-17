package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-logr/logr"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	requeuePeriodSeconds = 5
	timeFormat           = "2006-01-02_15-04-05.000000"
)

type CreateObjectFunction func() bundle.Object

func newGenericStatusSyncController(mgr ctrl.Manager, logName string, transport transport.Transport,
	finalizerName string, bundleKey string, createObjFunc CreateObjectFunction, syncInterval time.Duration) error {
	statusSyncCtrl := &genericStatusSyncController{
		client:               mgr.GetClient(),
		log:                  ctrl.Log.WithName(logName),
		transport:            transport,
		transportBundleKey:   bundleKey,
		finalizerName:        finalizerName,
		createObjFunc:        createObjFunc,
		periodicSyncInterval: syncInterval,
	}
	statusSyncCtrl.init()

	return ctrl.NewControllerManagedBy(mgr).For(createObjFunc()).Complete(statusSyncCtrl)
}

// TODO need to handle race condition in case of failure in controller
// what happens if the controller updates the bundle with deleted object, removes the finalizer but before the other
// thread sent the message using transport layer, the controller failed and had to restart?
// in this scenario the deletion update will be lost since the finalizer was removed and therefore the object was
// removed from the cluster. the controller will not get any more updates about this object.

// need to come up with solution for this issue - either to persist the bundle from memory to a volume on every change
// or to remove the finalizers from deleted objects only after the bundle with the deleted objects was sent.

type genericStatusSyncController struct {
	client               client.Client
	log                  logr.Logger
	transport            transport.Transport
	transportBundleKey   string
	bundle               *bundle.StatusBundle
	lastBundleTimestamp  time.Time
	finalizerName        string
	createObjFunc        CreateObjectFunction
	periodicSyncInterval time.Duration
	stopChan             chan struct{}
	startOnce            sync.Once
	stopOnce             sync.Once
}

func (c *genericStatusSyncController) init() {
	c.startOnce.Do(func() {
		c.bundle = bundle.NewStatusBundle()
		c.lastBundleTimestamp = *(c.bundle.GetBundleTimestamp())
		go c.periodicSync()
	})
}

func (c *genericStatusSyncController) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopChan)
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
		reqLogger.Info("Reconciliation failed: %v", err)
		return ctrl.Result{Requeue: true, RequeueAfter: requeuePeriodSeconds * time.Second}, err
	}
	if c.isObjectBeingDeleted(object) {
		if err = c.deleteObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			reqLogger.Info("Reconciliation failed: %v", err)
			return ctrl.Result{Requeue: true, RequeueAfter: requeuePeriodSeconds * time.Second}, err
		}
	} else { // otherwise, the object was not deleted and no error occurred
		if err = c.updateObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			reqLogger.Info("Reconciliation failed: %v", err)
			return ctrl.Result{Requeue: true, RequeueAfter: requeuePeriodSeconds * time.Second}, err
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
	c.bundle.UpdateObject(object)
	return nil
}

func (c *genericStatusSyncController) addFinalizer(ctx context.Context, object bundle.Object, log logr.Logger) error {
	if containsString(object.GetFinalizers(), c.finalizerName) {
		return nil
	}

	log.Info("adding finalizer")
	controllerutil.AddFinalizer(object, c.finalizerName)
	if err := c.client.Update(ctx, object); err != nil {
		return fmt.Errorf("failed to add a finalizer: %s", err)
	}
	return nil
}
func (c *genericStatusSyncController) deleteObjectAndFinalizer(ctx context.Context, object bundle.Object,
	log logr.Logger) error {
	c.bundle.DeleteObject(object)
	return c.removeFinalizer(ctx, object, log)
}

func (c *genericStatusSyncController) removeFinalizer(ctx context.Context, object bundle.Object, log logr.Logger) error {
	if containsString(object.GetFinalizers(), c.finalizerName) {
		return nil
	}

	log.Info("removing finalizer")
	controllerutil.RemoveFinalizer(object, c.finalizerName)
	if err := c.client.Update(ctx, object); err != nil {
		return fmt.Errorf("failed to remove a finalizer: %s", err)
	}
	return nil
}

func (c *genericStatusSyncController) periodicSync() {
	ticker := time.NewTicker(c.periodicSyncInterval)
	for {
		select {
		case <-c.stopChan:
			ticker.Stop()
			return
		case <-ticker.C:
			bundleTimestamp := c.bundle.GetBundleTimestamp()
			if bundleTimestamp.After(c.lastBundleTimestamp) { // send to transport only if bundle has changed
				c.syncToTransport(c.transportBundleKey, datatypes.StatusBundle, bundleTimestamp, c.bundle)
				c.lastBundleTimestamp = *bundleTimestamp
			}
		}
	}
}

func (c *genericStatusSyncController) syncToTransport(id string, objType string, timestamp *time.Time,
	payload *bundle.StatusBundle) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("failed to sync object from type %s with id %s- %s", objType, id, err)
		return
	}
	c.transport.SendAsync(id, objType, timestamp.Format(timeFormat), payloadBytes)
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}

	return false
}
