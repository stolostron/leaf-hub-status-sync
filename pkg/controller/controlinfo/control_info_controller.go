package controlinfo

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/controller/syncintervals"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

// LeafHubControlInfoController manages control info bundle traffic.
type LeafHubControlInfoController struct {
	log                     logr.Logger
	bundle                  bundle.Bundle
	transportBundleKey      string
	transport               transport.Transport
	resolveSyncIntervalFunc syncintervals.ResolveSyncIntervalFunc
}

// NewLeafHubControlInfoController creates a new instance of LeafHubControlInfoController.
func NewLeafHubControlInfoController(log logr.Logger, transport transport.Transport, leafHubName string,
	resolveSyncIntervalFunc syncintervals.ResolveSyncIntervalFunc) (*LeafHubControlInfoController, error) {
	transportBundleKey := fmt.Sprintf("%s.%s", leafHubName, datatypes.ControlInfoMsgKey)
	initialGeneration := helpers.GetGenerationFromTransport(transport, transportBundleKey, datatypes.StatusBundle)

	return &LeafHubControlInfoController{
		log:                     log.WithName("controlinfo"),
		bundle:                  bundle.NewControlInfoBundle(leafHubName, initialGeneration),
		transportBundleKey:      transportBundleKey,
		transport:               transport,
		resolveSyncIntervalFunc: resolveSyncIntervalFunc,
	}, nil
}

// Start function starts control info controller.
func (c *LeafHubControlInfoController) Start(stopChannel <-chan struct{}) error {
	ctx, cancelContext := context.WithCancel(context.Background())
	defer cancelContext()

	c.log.Info("started control info manager")

	go c.periodicSend(ctx)

	<-stopChannel // blocking wait for stop event
	c.log.Info("stopped control info manager")
	cancelContext()

	return nil
}

func (c *LeafHubControlInfoController) periodicSend(ctx context.Context) {
	currentSyncInterval := c.resolveSyncIntervalFunc()
	ticker := time.NewTicker(currentSyncInterval)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return

		case <-ticker.C: // wait for next time interval
			c.bundle.UpdateObject(nil)
			version := strconv.FormatUint(c.bundle.GetBundleGeneration(), helpers.Base10)

			if err := helpers.SyncToTransport(c.transport, c.transportBundleKey, datatypes.StatusBundle, version,
				c.bundle); err != nil {
				c.log.Info(fmt.Sprintf("failed to sync object from type %s with id %s- %s", datatypes.StatusBundle,
					c.transportBundleKey, err))
			}

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
