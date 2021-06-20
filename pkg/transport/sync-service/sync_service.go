package sync_service

import (
	"bytes"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/open-horizon/edge-sync-service-client/client"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"strconv"
	"sync"
)

const (
	syncServiceProtocol = "SYNC_SERVICE_PROTOCOL"
	syncServiceHost     = "SYNC_SERVICE_HOST"
	syncServicePort     = "SYNC_SERVICE_PORT"
)

type SyncService struct {
	client    *client.SyncServiceClient
	log       logr.Logger
	msgChan   chan *syncServiceMessage
	stopChan  chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewSyncService() (*SyncService, error) {
	log := ctrl.Log.WithName("sync-service")
	serverProtocol, host, port, err := readEnvVars(log)
	if err != nil {
		return nil, err
	}
	syncServiceClient := client.NewSyncServiceClient(serverProtocol, host, port)
	syncServiceClient.SetAppKeyAndSecret("user@myorg", "")
	return &SyncService{
		client:   syncServiceClient,
		log:      log,
		msgChan:  make(chan *syncServiceMessage),
		stopChan: make(chan struct{}, 1),
	}, nil
}

func readEnvVars(log logr.Logger) (string, string, uint16, error) {
	protocol := os.Getenv(syncServiceProtocol)
	if protocol == "" {
		return "", "", 0, fmt.Errorf("missing environment variable %s", syncServiceProtocol)
	}
	host := os.Getenv(syncServiceHost)
	if host == "" {
		return "", "", 0, fmt.Errorf("missing environment variable %s", syncServiceHost)
	}
	portStr := os.Getenv(syncServicePort)
	if portStr == "" {
		return "", "", 0, fmt.Errorf("missing environment variable %s", syncServicePort)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", "", 0, fmt.Errorf("environment variable %s is not int", syncServicePort)
	}
	return protocol, host, uint16(port), nil
}

func (s *SyncService) Start() {
	s.startOnce.Do(func() {
		go s.sendMessages()
	})
}

func (s *SyncService) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopChan)
	})
}

func (s *SyncService) SendAsync(id string, msgType string, version string, payload []byte) {
	message := &syncServiceMessage{
		id:      id,
		msgType: msgType,
		version: version,
		payload: payload,
	}
	s.msgChan <- message
}

func (s *SyncService) sendMessages() {
	for {
		select {
		case <-s.stopChan:
			return
		case msg := <-s.msgChan:
			metaData := client.ObjectMetaData{
				ObjectID:   msg.id,
				ObjectType: msg.msgType,
				Version:    msg.version,
			}
			err := s.client.UpdateObject(&metaData)
			if err != nil {
				s.log.Error(err, "Failed to update the object in the Edge Sync Service")
				continue
			}
			reader := bytes.NewReader(msg.payload)
			err = s.client.UpdateObjectData(&metaData, reader)
			if err != nil {
				s.log.Error(err, "Failed to update the object data in the Edge Sync Service")
				continue
			}
			s.log.Info(fmt.Sprintf("Message '%s' from type '%s' with version '%s' sent", msg.id, msg.msgType,
				msg.version))
		}
	}
}
