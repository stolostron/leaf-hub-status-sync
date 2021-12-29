package controlinfo

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	configv1 "github.com/open-cluster-management/hub-of-hubs-data-types/apis/config/v1"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	controlInfoLogName = "control-info"
)

// AddControlInfoController creates a new instance of control info controller and adds it to the manager.
func AddControlInfoController(mgr ctrl.Manager, transport transport.Transport, leafHubName string, incarnation uint64,
	_ *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals) error {
	transportBundleKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.ControlInfoMsgKey)

	controlInfoCtrl := &LeafHubControlInfoController{
		log:                     ctrl.Log.WithName(controlInfoLogName),
		bundle:                  bundle.NewControlInfoBundle(leafHubName, incarnation),
		transportBundleKey:      transportBundleKey,
		transport:               transport,
		resolveSyncIntervalFunc: syncIntervalsData.GetControlInfo,
	}

	if err := mgr.Add(controlInfoCtrl); err != nil {
		return fmt.Errorf("failed to add controller to the manager - %w", err)
	}

	return nil
}

// LeafHubControlInfoController manages control info bundle traffic.
type LeafHubControlInfoController struct {
	log                     logr.Logger
	bundle                  bundle.Bundle
	transportBundleKey      string
	transport               transport.Transport
	resolveSyncIntervalFunc syncintervals.ResolveSyncIntervalFunc
}

// Start function starts control info controller.
func (c *LeafHubControlInfoController) Start(stopChannel <-chan struct{}) error {
	ctx, cancelContext := context.WithCancel(context.Background())
	defer cancelContext()

	c.log.Info("Starting Controller")

	go c.periodicSync(ctx)

	<-stopChannel // blocking wait for stop event
	c.log.Info("Stopping Controller")

	return nil
}

func (c *LeafHubControlInfoController) periodicSync(ctx context.Context) {
	currentSyncInterval := c.resolveSyncIntervalFunc()
	ticker := time.NewTicker(currentSyncInterval)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return

		case <-ticker.C: // wait for next time interval
			c.syncBundle()

			resolvedInterval := c.resolveSyncIntervalFunc()

			// reset ticker if sync interval has changed
			if resolvedInterval != currentSyncInterval {
				currentSyncInterval = resolvedInterval
				ticker.Reset(currentSyncInterval)
				c.log.Info(fmt.Sprintf("sync interval has been reset to %s", currentSyncInterval.String()))
			}
		}
	}
}

func (c *LeafHubControlInfoController) syncBundle() {
	c.bundle.UpdateObject(nil) // increase generation

	if err := helpers.SyncToTransport(c.transport, c.transportBundleKey, datatypes.StatusBundle,
		c.bundle.GetBundleVersion(), c.bundle); err != nil {
		c.log.Error(err, "failed to sync to transport")
	}
}
