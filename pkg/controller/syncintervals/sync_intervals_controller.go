package syncintervals

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	configMapName = "sync-intervals"
)

// AddSyncIntervalsController creates a new instance of config map controller and adds it to the manager.
func AddSyncIntervalsController(mgr ctrl.Manager, logName string, syncIntervals *SyncIntervals) error {
	syncIntervalsCtrl := &syncIntervalsController{
		client:            mgr.GetClient(),
		log:               ctrl.Log.WithName(logName),
		syncIntervalsData: syncIntervals,
	}

	syncIntervalsPredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return meta.GetNamespace() == datatypes.HohSystemNamespace && meta.GetName() == configMapName
	})

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&v1.ConfigMap{}).
		WithEventFilter(syncIntervalsPredicate).
		Complete(syncIntervalsCtrl); err != nil {
		return fmt.Errorf("failed to add controller to the manager - %w", err)
	}

	return nil
}

type syncIntervalsController struct {
	client            client.Client
	log               logr.Logger
	syncIntervalsData *SyncIntervals
}

func (c *syncIntervalsController) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	ctx := context.Background()
	configMap := &v1.ConfigMap{}

	if err := c.client.Get(ctx, request.NamespacedName, configMap); apierrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
		return ctrl.Result{Requeue: true, RequeueAfter: helpers.RequeuePeriodSeconds * time.Second},
			fmt.Errorf("reconciliation failed: %w", err)
	}

	c.setSyncInterval(reqLogger, configMap, "managed_clusters", &c.syncIntervalsData.managedClusters)
	c.setSyncInterval(reqLogger, configMap, "policies", &c.syncIntervalsData.policies)

	reqLogger.Info("Reconciliation complete.")

	return ctrl.Result{}, nil
}

func (c *syncIntervalsController) setSyncInterval(log logr.Logger, configMap *v1.ConfigMap, key string,
	syncInterval *time.Duration) {
	intervalStr, found := configMap.Data[key]
	if !found {
		log.Info(fmt.Sprintf("%s sync interval not defined, using %s", key, syncInterval.String()))
		return
	}

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		log.Info(fmt.Sprintf("%s sync interval has invalid format, using %s", key, syncInterval.String()))
		return
	}

	*syncInterval = interval
}
