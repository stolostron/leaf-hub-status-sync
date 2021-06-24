package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-logr/logr"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strconv"
	"sync"
	"time"
)

const (
	requeuePeriodSeconds = 5
)

type CreateObjectFunction func() bundle.Object

var genericPredicate = &GenericPredicate{}

func newGenericStatusSyncController(mgr ctrl.Manager, logName string, transport transport.Transport,
	finalizerName string, bundleKey string, createObjFunc CreateObjectFunction, syncInterval time.Duration,
	leafHubName string, filterWithPredicate bool) error {
	statusSyncCtrl := &genericStatusSyncController{
		client:               mgr.GetClient(),
		log:                  ctrl.Log.WithName(logName),
		transport:            transport,
		transportBundleKey:   bundleKey,
		leafHubName:          leafHubName,
		finalizerName:        finalizerName,
		createObjFunc:        createObjFunc,
		periodicSyncInterval: syncInterval,
		filterWithPredicate:  filterWithPredicate,
	}
	statusSyncCtrl.init()

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).For(createObjFunc())
	if filterWithPredicate {
		controllerBuilder = controllerBuilder.WithEventFilter(genericPredicate)
	}
	return controllerBuilder.Complete(statusSyncCtrl)
}

type genericStatusSyncController struct {
	client                   client.Client
	log                      logr.Logger
	transport                transport.Transport
	transportBundleKey       string
	leafHubName              string
	bundle                   *bundle.StatusBundle
	lastSentBundleGeneration uint64
	finalizerName            string
	createObjFunc            CreateObjectFunction
	periodicSyncInterval     time.Duration
	startOnce                sync.Once
	filterWithPredicate      bool
}

func (c *genericStatusSyncController) init() {
	c.startOnce.Do(func() {
		c.bundle = bundle.NewStatusBundle(c.leafHubName)
		c.lastSentBundleGeneration = c.bundle.GetBundleGeneration()
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
		return ctrl.Result{Requeue: true, RequeueAfter: requeuePeriodSeconds * time.Second}, err
	}
	if c.isObjectBeingDeleted(object) {
		if err = c.deleteObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
			return ctrl.Result{Requeue: true, RequeueAfter: requeuePeriodSeconds * time.Second}, err
		}
	} else { // otherwise, the object was not deleted and no error occurred
		if err = c.updateObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
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
	cleanObject(object)
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
		case <-ticker.C:
			bundleGeneration := c.bundle.GetBundleGeneration()
			if bundleGeneration > c.lastSentBundleGeneration { // send to transport only if bundle has changed
				c.syncToTransport(fmt.Sprintf("%s.%s", c.leafHubName, c.transportBundleKey),
					datatypes.StatusBundle, strconv.FormatUint(bundleGeneration, 10), c.bundle)
				c.lastSentBundleGeneration = bundleGeneration
			}
		}
	}
}

func (c *genericStatusSyncController) syncToTransport(id string, objType string, generation string,
	payload *bundle.StatusBundle) {
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

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}

	return false
}
