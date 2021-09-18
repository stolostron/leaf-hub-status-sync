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

const (
	defaultPeriodSeconds = 60
	defaultGeneration    = 0
)

// LeafHubControlInfoController manages control info bundle traffic.
type LeafHubControlInfoController struct {
	transport transport.Transport
	bundle    *bundle.ControlInfoBundle
	log       logr.Logger
}

// NewLeafHubControlInfoController creates a new instance of LeafHubControlInfoController.
func NewLeafHubControlInfoController(transport transport.Transport,
	leafHubName string, log logr.Logger) (*LeafHubControlInfoController, error) {
	return &LeafHubControlInfoController{
		transport: transport,
		bundle:    &bundle.ControlInfoBundle{LeafHubName: leafHubName},
		log:       log.WithName("controlinfo"),
	}, nil
}

// Start function starts control info controller.
func (m *LeafHubControlInfoController) Start(stopChannel <-chan struct{}) error {
	ctx, cancelContext := context.WithCancel(context.Background())
	defer cancelContext()

	m.log.Info("started control info manager")

	go m.periodicSend(ctx)

	for {
		<-stopChannel // blocking wait for stop event
		m.log.Info("stopped control info manager")
		cancelContext()

		return nil
	}
}

func (m *LeafHubControlInfoController) periodicSend(ctx context.Context) {
	ticker := time.NewTicker(defaultPeriodSeconds * time.Second)
	id := fmt.Sprintf("%s.%s", m.bundle.LeafHubName, datatypes.ControlInfoMsgKey)
	version := strconv.FormatUint(defaultGeneration, helpers.Base10)

	payload, err := json.Marshal(m.bundle)
	if err != nil {
		m.log.Info(fmt.Sprintf("failed to sync object from type %s with id %s- %s", datatypes.StatusBundle, id, err))
		ticker.Stop()

		return
	}

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return

		case <-ticker.C: // wait for next time interval
			m.transport.SendAsync(id, datatypes.StatusBundle, version, payload)
		}
	}
}
