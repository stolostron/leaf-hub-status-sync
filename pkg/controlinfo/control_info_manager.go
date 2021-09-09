package controlinfo

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-logr/logr"
	datatypes "github.com/open-cluster-management/hub-of-hubs-data-types"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/bundle"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/helpers"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

const (
	// PeriodSeconds is the time to wait before sending next control info bundle.
	PeriodSeconds = 60
)

// Manager manages control info bundle traffic.
type Manager struct {
	transport   transport.Transport
	leafHubName string
	log         logr.Logger
	stopChan    chan bool
	startOnce   sync.Once
	stopOnce    sync.Once
}

// NewManager creates a new instance of ControlInfoManager.
func NewManager(transport transport.Transport, leafHubName string, log logr.Logger) (*Manager, error) {
	return &Manager{
		transport:   transport,
		leafHubName: leafHubName,
		log:         log.WithName("control-info"),
		stopChan:    make(chan bool),
	}, nil
}

// Start function starts control info manager.
func (m *Manager) Start() {
	m.log.Info("started ControlInfoManager")

	m.startOnce.Do(func() {
		go m.periodicSend()
	})
}

// Stop function stops control info manager.
func (m *Manager) Stop() {
	m.log.Info("stopped ControlInfoManager")

	m.stopOnce.Do(func() {
		m.stopChan <- true
	})
}

func (m *Manager) periodicSend() {
	ticker := time.NewTicker(PeriodSeconds * time.Second)
	id := fmt.Sprintf("%s.%s", m.leafHubName, datatypes.ControlInfoKey)
	generation := helpers.GetBundleGenerationFromTransport(m.transport, id, datatypes.StatusBundle)
	version := strconv.FormatUint(generation, helpers.Base10)

	payload, err := json.Marshal(&bundle.ControlInfoBundle{LeafHubName: m.leafHubName})
	if err != nil {
		m.log.Info(fmt.Sprintf("failed to sync object from type %s with id %s- %s", datatypes.StatusBundle, id, err))
		ticker.Stop()

		return
	}

	for {
		select {
		case <-m.stopChan:
			ticker.Stop()
			return

		case <-ticker.C: // wait for next time interval
			m.transport.SendAsync(id, datatypes.StatusBundle, version, payload)
		}
	}
}
