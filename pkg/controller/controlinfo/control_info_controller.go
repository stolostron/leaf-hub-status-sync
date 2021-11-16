package controlinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

const defaultPeriodSeconds = 60

// LeafHubControlInfoController manages control info bundle traffic.
type LeafHubControlInfoController struct {
	log       logr.Logger
	bundle    *bundle.ControlInfoBundle
	transport transport.Transport
}

// NewLeafHubControlInfoController creates a new instance of LeafHubControlInfoController.
func NewLeafHubControlInfoController(log logr.Logger, transport transport.Transport,
	leafHubName string) (*LeafHubControlInfoController, error) {
	return &LeafHubControlInfoController{
		log:       log.WithName("controlinfo"),
		bundle:    &bundle.ControlInfoBundle{LeafHubName: leafHubName, Generation: 0},
		transport: transport,
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
	ticker := time.NewTicker(defaultPeriodSeconds * time.Second)
	id := fmt.Sprintf("%s.%s", c.bundle.LeafHubName, datatypes.ControlInfoMsgKey)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return

		case <-ticker.C: // wait for next time interval
			c.bundle.Generation++
			version := strconv.FormatUint(c.bundle.Generation, helpers.Base10)

			payload, err := json.Marshal(c.bundle)
			if err != nil {
				c.log.Info(fmt.Sprintf("failed to sync object from type %s with id %s- %s", datatypes.StatusBundle, id, err))
			}

			c.transport.SendAsync(id, datatypes.StatusBundle, version, payload)
		}
	}
}
