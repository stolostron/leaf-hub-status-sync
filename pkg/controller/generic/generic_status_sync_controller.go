package generic

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	retryhandler "github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/generic/sync-retry-handler"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// CreateObjectFunction is a function for how to create an object that is stored inside the bundle.
type CreateObjectFunction func() bundle.Object

// NewGenericStatusSyncController creates a new instance of genericStatusSyncController and adds it to the manager.
func NewGenericStatusSyncController(mgr ctrl.Manager, logName string, transport transport.Transport,
	finalizerName string, orderedBundleCollection []*BundleCollectionEntry, createObjFunc CreateObjectFunction,
	predicate predicate.Predicate, transportRetryChan chan *transport.Message,
	resolveSyncIntervalFunc syncintervals.ResolveSyncIntervalFunc) error {
	statusSyncCtrl := &genericStatusSyncController{
		client:                  mgr.GetClient(),
		log:                     ctrl.Log.WithName(logName),
		transport:               transport,
		transportRetryChan:      transportRetryChan,
		orderedBundleCollection: orderedBundleCollection,
		finalizerName:           finalizerName,
		createObjFunc:           createObjFunc,
		resolveSyncIntervalFunc: resolveSyncIntervalFunc,
		lock:                    sync.Mutex{},
	}
	statusSyncCtrl.init()

	controllerBuilder := ctrl.NewControllerManagedBy(mgr).For(createObjFunc())
	if predicate != nil {
		controllerBuilder = controllerBuilder.WithEventFilter(predicate)
	}

	if err := controllerBuilder.Complete(statusSyncCtrl); err != nil {
		return fmt.Errorf("failed to add controller to the manager - %w", err)
	}

	return nil
}

type genericStatusSyncController struct {
	client                  client.Client
	log                     logr.Logger
	transport               transport.Transport
	transportRetryChan      chan *transport.Message
	orderedBundleCollection []*BundleCollectionEntry
	finalizerName           string
	createObjFunc           CreateObjectFunction
	resolveSyncIntervalFunc syncintervals.ResolveSyncIntervalFunc
	startOnce               sync.Once
	lock                    sync.Mutex
}

func (c *genericStatusSyncController) init() {
	c.startOnce.Do(func() {
		go c.periodicSync()
		go c.handleRetries()
	})
}

func (c *genericStatusSyncController) handleRetries() {
	// retry attempts counter map for exponential backoff
	retryHandlersMap := make(map[string]*retryhandler.SyncRetryHandler)
	// create a handler for each bundle with the retryOperation set as bundle syncing and start the handler
	for _, entry := range c.orderedBundleCollection {
		entry := entry // to use in func
		retryHandlersMap[entry.transportBundleKey] = retryhandler.NewSyncRetryHandler(func(transportMsg interface{}) {
			// check if it meets retry pre-attempt conditions
			if !entry.deliveryRegistration.CheckEventCondition(transport.BeforeDeliveryRetry, transport.ArgTypeNone,
				nil) {
				return
			}

			transportMessage, _ := transportMsg.(*transport.Message)

			// check if it meets version conditions
			if entry.deliveryRegistration.CheckEventCondition(transport.BeforeDeliveryRetry,
				transport.ArgTypeBundleVersion, transportMessage.Version) {
				// message should be retried, it is the most recent of this type
				c.log.Info("retrying bundle delivery", "MessageId", transportMessage.ID,
					"MessageType", transportMessage.MsgType, "Version")
				// retry
				c.transport.SendAsync(transportMessage)
			}
		}).Start(context.Background(), entry.deliveryRegistration)
	}

	for {
		transportMessage := <-c.transportRetryChan
		retryHandlersMap[transportMessage.ID].HandleFailure(transportMessage)
	}
}

func (c *genericStatusSyncController) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	ctx := context.Background()
	object := c.createObjFunc()

	if err := c.client.Get(ctx, request.NamespacedName, object); apierrors.IsNotFound(err) {
		// the instance was deleted and it had no finalizer on it.
		// this means either LH removed the finalizer so it was already deleted from bundle, or
		// LH didn't update HoH about this object ever.
		// either way, no need to do anything in this state.
		return ctrl.Result{}, nil
	} else if err != nil {
		reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
		return ctrl.Result{Requeue: true, RequeueAfter: helpers.RequeuePeriod},
			fmt.Errorf("reconciliation failed: %w", err)
	}

	if c.isObjectBeingDeleted(object) {
		if err := c.deleteObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
			return ctrl.Result{Requeue: true, RequeueAfter: helpers.RequeuePeriod}, err
		}
	} else { // otherwise, the object was not deleted and no error occurred
		if err := c.updateObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
			return ctrl.Result{Requeue: true, RequeueAfter: helpers.RequeuePeriod}, err
		}
	}

	reqLogger.Info("Reconciliation complete.")

	return ctrl.Result{}, nil
}

func (c *genericStatusSyncController) isObjectBeingDeleted(object bundle.Object) bool {
	return !object.GetDeletionTimestamp().IsZero()
}

func (c *genericStatusSyncController) updateObjectAndFinalizer(ctx context.Context, object bundle.Object,
	log logr.Logger) error {
	if err := c.addFinalizer(ctx, object, log); err != nil {
		return fmt.Errorf("failed to add finalizer - %w", err)
	}

	cleanObject(object)

	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Unlock()

	for _, entry := range c.orderedBundleCollection {
		entry.bundle.UpdateObject(object) // update in each bundle from the collection according to their order.
	}

	return nil
}

func (c *genericStatusSyncController) addFinalizer(ctx context.Context, object bundle.Object, log logr.Logger) error {
	if controllerutil.ContainsFinalizer(object, c.finalizerName) {
		return nil
	}

	log.Info("adding finalizer")
	controllerutil.AddFinalizer(object, c.finalizerName)

	if err := c.client.Update(ctx, object); err != nil {
		return fmt.Errorf("failed to add finalizer %s - %w", c.finalizerName, err)
	}

	return nil
}

func (c *genericStatusSyncController) deleteObjectAndFinalizer(ctx context.Context, object bundle.Object,
	log logr.Logger) error {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync

	for _, entry := range c.orderedBundleCollection {
		entry.bundle.DeleteObject(object) // delete from all bundles.
	}

	c.lock.Unlock() // not using defer since remove finalizer may get delayed. release lock as soon as possible.

	return c.removeFinalizer(ctx, object, log)
}

func (c *genericStatusSyncController) removeFinalizer(ctx context.Context, object bundle.Object,
	log logr.Logger) error {
	if !controllerutil.ContainsFinalizer(object, c.finalizerName) {
		return nil // if finalizer is not there, do nothing.
	}

	log.Info("removing finalizer")
	controllerutil.RemoveFinalizer(object, c.finalizerName)

	if err := c.client.Update(ctx, object); err != nil {
		return fmt.Errorf("failed to remove finalizer %s - %w", c.finalizerName, err)
	}

	return nil
}

func (c *genericStatusSyncController) periodicSync() {
	currentSyncInterval := c.resolveSyncIntervalFunc()
	ticker := time.NewTicker(currentSyncInterval)

	for {
		<-ticker.C // wait for next time interval
		c.syncBundles()

		resolvedInterval := c.resolveSyncIntervalFunc()

		// reset ticker if sync interval has changed
		if resolvedInterval != currentSyncInterval {
			currentSyncInterval = resolvedInterval
			ticker.Reset(currentSyncInterval)
			c.log.Info(fmt.Sprintf("sync interval has been reset to %s", currentSyncInterval.String()))
		}
	}
}

func (c *genericStatusSyncController) syncBundles() {
	c.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer c.lock.Unlock()

	for _, entry := range c.orderedBundleCollection {
		// evaluate if bundle has to be sent only if delivery pre-attempt conditions are met
		if !entry.deliveryRegistration.CheckEventCondition(transport.BeforeDeliveryAttempt, transport.ArgTypeNone,
			nil) {
			continue
		}

		bundleVersion := entry.bundle.GetBundleVersion()

		// send to transport only if delivery pre-attempt generation conditions are met
		if entry.deliveryRegistration.CheckEventCondition(transport.BeforeDeliveryAttempt,
			transport.ArgTypeBundleVersion, bundleVersion) {
			if err := helpers.SyncToTransport(c.transport, entry.transportBundleKey, datatypes.StatusBundle,
				bundleVersion, entry.bundle); err != nil {
				c.log.Error(err, "failed to sync to transport")

				continue // do not update last sent generation in case of failure in sync bundle to transport
				// the transport component is responsible for triggering another retry
			}

			// update last-sent-version and invoke delegate actions for delivery attempt event
			entry.deliveryRegistration.SetLastSentBundleVersion(bundleVersion)
			// inform delivery registration of transportation attempt
			entry.deliveryRegistration.InvokeEventActions(transport.AfterDeliveryAttempt,
				transport.ArgTypeNone, nil, true) // call is blocking so further updates don't go through until handled
		}
	}
}

func cleanObject(object bundle.Object) {
	object.SetManagedFields(nil)
	object.SetFinalizers(nil)
	object.SetGeneration(0)
	object.SetOwnerReferences(nil)
	object.SetSelfLink("")
	object.SetClusterName("")
}
