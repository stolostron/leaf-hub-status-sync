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

func NewSyncService() *SyncService {
	log := ctrl.Log.WithName("sync-service")
	serverProtocol, host, port := readEnvVars(log)
	syncServiceClient := client.NewSyncServiceClient(serverProtocol, host, port)
	syncServiceClient.SetAppKeyAndSecret("user@myorg", "")
	return &SyncService{
		client:   syncServiceClient,
		log:      log,
		msgChan:  make(chan *syncServiceMessage),
		stopChan: make(chan struct{}, 1),
	}
}

func readEnvVars(log logr.Logger) (string, string, uint16) {
	protocol := os.Getenv(syncServiceProtocol)
	if protocol == "" {
		log.Error(fmt.Errorf("missing environment variable %s", syncServiceProtocol), "failed to initialize")
	}
	host := os.Getenv(syncServiceHost)
	if host == "" {
		log.Error(fmt.Errorf("missing environment variable %s", syncServiceHost), "failed to initialize")
	}
	portStr := os.Getenv(syncServicePort)
	if portStr == "" {
		log.Error(fmt.Errorf("missing environment variable %s", syncServicePort), "failed to initialize")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Error(fmt.Errorf("environment variable %s is not int", syncServicePort), "failed to initialize")
	}
	return protocol, host, uint16(port)
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

// if the object doesn't exist or an error occurred returns an empty string
func (s *SyncService) GetVersion(id string, msgType string) string {
	objectMetadata, err := s.client.GetObjectMetadata(msgType, id)
	if err != nil {
		return ""
	}
	return objectMetadata.Version
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
			s.log.Info("Message '%v' from type '%v' with version '%v' sent", msg.id, msg.msgType, msg.version)
		}
	}
}
