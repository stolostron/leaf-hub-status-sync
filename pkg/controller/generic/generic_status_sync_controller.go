package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/go-logr/logr"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	// RequeuePeriodSeconds is the time to wait until reconciliation retry in failure cases.
	RequeuePeriodSeconds          = 5
	decimal                       = 10
	uint64Size                    = 64
	exponentialBackOffBase        = 2
	exponentialBackOffAttemptsCap = 9
	exponentialBackOffTimeUnitMS  = 200 * 1000

	// envNumberOfSimulatedLeafHubs is environment variable used to control number of simulated leaf hubs.
	envNumberOfSimulatedLeafHubs = "NUMBER_OF_SIMULATED_LEAF_HUBS"
	// transportBundleKeyParts is a number of parts in BundleCollectionEntry.transportBundleKey field.
	transportBundleKeyParts = 2
)

// CreateObjectFunction is a function for how to create an object that is stored inside the bundle.
type CreateObjectFunction func() bundle.Object

// NewGenericStatusSyncController creates a new instance of genericStatusSyncController and adds it to the manager.
func NewGenericStatusSyncController(mgr ctrl.Manager, logName string, incarnation uint64, transport transport.Transport,
	finalizerName string, orderedBundleCollection []*BundleCollectionEntry, createObjFunc CreateObjectFunction,
	syncInterval time.Duration, transportRetryChan chan *transport.Message, predicate predicate.Predicate) error {
	statusSyncCtrl := &genericStatusSyncController{
		incarnation:             incarnation,
		client:                  mgr.GetClient(),
		log:                     ctrl.Log.WithName(logName),
		transport:               transport,
		transportRetryChan:      transportRetryChan,
		orderedBundleCollection: orderedBundleCollection,
		finalizerName:           finalizerName,
		createObjFunc:           createObjFunc,
		periodicSyncInterval:    syncInterval,
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

type simulationContext struct {
	numOfLeafHubs int
	mutex         sync.Mutex
}

func newSimulationContext(log logr.Logger) *simulationContext {
	envNumOfSimulateLeafHubs, found := os.LookupEnv(envNumberOfSimulatedLeafHubs)

	if found {
		if value, err := strconv.Atoi(envNumOfSimulateLeafHubs); err != nil {
			log.Info(fmt.Sprintf("Failed to convert environment variable '%s', value: %s, err: %s",
				envNumberOfSimulatedLeafHubs, envNumOfSimulateLeafHubs, err))
		} else {
			switch {
			case value >= 0:
				return &simulationContext{numOfLeafHubs: value}
			default:
				log.Info(fmt.Sprintf("Environment variable '%s' must be a non-negative integer value, provided value '%s'.",
					envNumberOfSimulatedLeafHubs, envNumOfSimulateLeafHubs))
			}
		}
	} else {
		log.Info(fmt.Sprintf("Environment variable '%s' is not defined", envNumberOfSimulatedLeafHubs))
	}

	return &simulationContext{numOfLeafHubs: 0}
}

type genericStatusSyncController struct {
	incarnation             uint64
	client                  client.Client
	log                     logr.Logger
	transport               transport.Transport
	transportRetryChan      chan *transport.Message
	orderedBundleCollection []*BundleCollectionEntry
	finalizerName           string
	createObjFunc           CreateObjectFunction
	periodicSyncInterval    time.Duration
	startOnce               sync.Once
	simulationContext       *simulationContext
}

func (c *genericStatusSyncController) init() {
	c.simulationContext = newSimulationContext(c.log)
	c.startOnce.Do(func() {
		go c.periodicSync()
		go c.handleRetries()
	})
}

func (c *genericStatusSyncController) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	c.simulationContext.mutex.Lock()
	defer c.simulationContext.mutex.Unlock()
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
		return ctrl.Result{Requeue: true, RequeueAfter: RequeuePeriodSeconds * time.Second},
			fmt.Errorf("reconciliation failed: %w", err)
	}

	if c.isObjectBeingDeleted(object) {
		if err := c.deleteObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
			return ctrl.Result{Requeue: true, RequeueAfter: RequeuePeriodSeconds * time.Second}, err
		}
	} else { // otherwise, the object was not deleted and no error occurred
		if err := c.updateObjectAndFinalizer(ctx, object, reqLogger); err != nil {
			reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
			return ctrl.Result{Requeue: true, RequeueAfter: RequeuePeriodSeconds * time.Second}, err
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

	for _, entry := range c.orderedBundleCollection {
		entry.bundle.UpdateObject(object) // update in each bundle from the collection according to their order
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
	for _, entry := range c.orderedBundleCollection {
		entry.bundle.DeleteObject(object) // delete from all bundles
	}

	return c.removeFinalizer(ctx, object, log)
}

func (c *genericStatusSyncController) removeFinalizer(ctx context.Context, object bundle.Object,
	log logr.Logger) error {
	if !controllerutil.ContainsFinalizer(object, c.finalizerName) {
		return nil // if finalizer is not there, do nothing
	}

	log.Info("removing finalizer")
	controllerutil.RemoveFinalizer(object, c.finalizerName)

	if err := c.client.Update(ctx, object); err != nil {
		return fmt.Errorf("failed to remove finalizer %s - %w", c.finalizerName, err)
	}

	return nil
}

func (c *genericStatusSyncController) periodicSync() {
	ticker := time.NewTicker(c.periodicSyncInterval)

	for {
		<-ticker.C // wait for next time interval
		c.syncBundles()
	}
}

func (c *genericStatusSyncController) handleRetries() {
	// retry attempts counter map for exponential backoff
	retryCounterMap := make(map[string]*int)
	// add reset actions on delivery success for all bundles
	for _, entry := range c.orderedBundleCollection {
		retryCounter := 0
		retryCounterMap[entry.transportBundleKey] = &retryCounter

		entry.deliveryRegistration.AddAction(transport.DeliverySuccess, transport.ArgTypeNone,
			func(interface{}) {
				retryCounter = 0
			})
	}

	for {
		transportMessage := <-c.transportRetryChan
		// find the entry that is responsible for the bundle to be retried
		for _, entry := range c.orderedBundleCollection {
			if entry.transportBundleKey == transportMessage.ID {
				// check if it meets retry pre-attempt conditions
				if !entry.deliveryRegistration.CheckEventCondition(transport.BeforeDeliveryRetry, transport.ArgTypeNone,
					nil) {
					break
				}

				// get generation
				messageGeneration, err := strconv.ParseUint(transportMessage.Version, decimal, uint64Size)
				if err != nil {
					c.log.Error(err, "failed to handle retry: illegal message version",
						"message id", transportMessage.ID, "message type", transportMessage.MsgType,
						"message version", transportMessage.Version)

					break
				}

				// exponential backoff
				go func() {
					backOffScale := helpers.Min(*retryCounterMap[transportMessage.ID], exponentialBackOffAttemptsCap)
					backOffDuration := time.Duration(math.Pow(exponentialBackOffBase,
						float64(backOffScale)) * exponentialBackOffTimeUnitMS)
					time.Sleep(backOffDuration)

					if entry.deliveryRegistration.CheckEventCondition(transport.BeforeDeliveryRetry,
						transport.ArgTypeBundleGeneration, messageGeneration) {
						// message should be retried, it is the most recent of this type
						c.log.Info("retrying bundle delivery", "bundle id", transportMessage.ID,
							"bundle version", transportMessage.Version,
							"attempt", *retryCounterMap[transportMessage.ID])
						c.transport.SendAsync(transportMessage)
					}
				}()
			}
		}
	}
}

func (c *genericStatusSyncController) syncBundles() {
	for _, entry := range c.orderedBundleCollection {
		// evaluate if bundle has to be sent only if delivery pre-attempt conditions are met
		if !entry.deliveryRegistration.CheckEventCondition(transport.BeforeDeliveryAttempt, transport.ArgTypeNone, nil) {
			continue
		}

		c.simulationContext.mutex.Lock()

		bundleGeneration := entry.bundle.GetBundleGeneration()

		// send to transport only if delivery pre-attempt generation conditions are met
		if entry.deliveryRegistration.CheckEventCondition(transport.BeforeDeliveryAttempt,
			transport.ArgTypeBundleGeneration, bundleGeneration) {
			c.syncToTransport(entry.transportBundleKey, datatypes.StatusBundle, bundleGeneration, entry.bundle)

			leafHubName := getLeafHubName(entry)
			// send simulated entries
			for i := 1; i <= c.simulationContext.numOfLeafHubs; i++ {
				simulatedLeafHubName := fmt.Sprintf("%s_simulated_%d", leafHubName, i)

				c.changeLeafHubName(entry, simulatedLeafHubName)

				c.syncToTransport(entry.transportBundleKey, datatypes.StatusBundle, bundleGeneration, entry.bundle)
			}
			// restore original leaf hub name for the entry
			c.changeLeafHubName(entry, leafHubName)

			// update last-sent-gen and invoke delegate actions for delivery attempt event
			entry.deliveryRegistration.UpdateSentGeneration(bundleGeneration)
			entry.deliveryRegistration.InvokeEventActions(transport.AfterDeliveryAttempt,
				transport.ArgTypeBundleObjectsCount, entry.bundle.GetObjectsCount())
		}

		c.simulationContext.mutex.Unlock()
	}
}

func (c *genericStatusSyncController) syncToTransport(id string, objType string, generation uint64,
	payload bundle.Bundle) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		c.log.Info(fmt.Sprintf("failed to sync object from type %s with id %s- %s", objType, id, err))
		return
	}

	transportMessage := &transport.Message{
		ID:      id,
		MsgType: objType,
		Version: fmt.Sprintf("%d.%d", c.incarnation, generation), // TODO: limit to sync-service
		Payload: payloadBytes,
	}

	c.transport.SendAsync(transportMessage)
}

func cleanObject(object bundle.Object) {
	object.SetManagedFields(nil)
	object.SetFinalizers(nil)
	object.SetGeneration(0)
	object.SetOwnerReferences(nil)
	object.SetSelfLink("")
	object.SetClusterName("")
}

func getLeafHubNameFieldPointer(entry *BundleCollectionEntry) *string {
	ptrToBundle := reflect.ValueOf(entry.bundle)
	reflectedBundle := reflect.Indirect(ptrToBundle)
	privateMember := reflectedBundle.FieldByName("LeafHubName")

	return (*string)(unsafe.Pointer(privateMember.UnsafeAddr()))
}

func getLeafHubName(entry *BundleCollectionEntry) string {
	return *getLeafHubNameFieldPointer(entry)
}

func (c *genericStatusSyncController) changeLeafHubName(entry *BundleCollectionEntry, newLeafHubName string) {
	tokens := strings.Split(entry.transportBundleKey, ".")

	if len(tokens) != transportBundleKeyParts {
		c.log.Info(fmt.Sprintf("unable to parse transportBundleKey '%s'", entry.transportBundleKey))
		return
	}

	// change transport bundle key as it depends on leaf hub name
	entry.transportBundleKey = fmt.Sprintf("%s.%s", newLeafHubName, tokens[1])

	// change bundle's 'leafHubName' field value
	realPtrToLeafHubName := getLeafHubNameFieldPointer(entry)
	*realPtrToLeafHubName = newLeafHubName
}
