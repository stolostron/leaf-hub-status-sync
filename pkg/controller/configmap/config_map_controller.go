package configmap

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	// RequeuePeriodSeconds is the time to wait until reconciliation retry in failure cases.
	RequeuePeriodSeconds = 5
	configMapName        = "leaf-hub-periodic-sync-intervals"
)

// AddConfigMapController creates a new instance of config map controller and adds it to the manager.
func AddConfigMapController(mgr ctrl.Manager, logName string, configMapData *HohConfigMapData) error {
	configMapCtrl := &configMapController{
		client:        mgr.GetClient(),
		log:           ctrl.Log.WithName(logName),
		configMapData: configMapData,
	}

	hohNamespacePredicate := predicate.NewPredicateFuncs(func(meta metav1.Object, object runtime.Object) bool {
		return meta.GetNamespace() == datatypes.HohSystemNamespace && meta.GetName() == configMapName
	})

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&v1.ConfigMap{}).
		WithEventFilter(hohNamespacePredicate).
		Complete(configMapCtrl); err != nil {
		return fmt.Errorf("failed to add controller to the manager - %w", err)
	}

	return nil
}

type configMapController struct {
	client        client.Client
	log           logr.Logger
	configMapData *HohConfigMapData
}

func (c *configMapController) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	reqLogger := c.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)

	ctx := context.Background()
	configMap := &v1.ConfigMap{}

	if err := c.client.Get(ctx, request.NamespacedName, configMap); apierrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	} else if err != nil {
		reqLogger.Info(fmt.Sprintf("Reconciliation failed: %s", err))
		return ctrl.Result{Requeue: true, RequeueAfter: RequeuePeriodSeconds * time.Second},
			fmt.Errorf("reconciliation failed: %w", err)
	}

	c.setPeriodicSyncInterval(reqLogger, configMap, "managed_clusters", &c.configMapData.Intervals.ManagedClusters)
	c.setPeriodicSyncInterval(reqLogger, configMap, "policies", &c.configMapData.Intervals.Policies)

	reqLogger.Info("Reconciliation complete.")

	return ctrl.Result{}, nil
}

func (c *configMapController) setPeriodicSyncInterval(log logr.Logger, configMap *v1.ConfigMap,
	key string, syncInterval *time.Duration) {
	if intervalStr, found := configMap.Data[key]; !found {
		log.Info(fmt.Sprintf("%s periodic sync interval not defined, using %s", key, *syncInterval))
	} else {
		if interval, err := time.ParseDuration(intervalStr); err != nil {
			log.Info(fmt.Sprintf("%s periodic sync interval has invalid format, using %s", key, *syncInterval))
		} else {
			*syncInterval = interval
		}
	}
}
