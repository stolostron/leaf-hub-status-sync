package syncservice

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/go-logr/logr"
	"github.com/open-horizon/edge-sync-service-client/client"
)

const (
	envVarSyncServiceProtocol = "SYNC_SERVICE_PROTOCOL"
	envVarSyncServiceHost     = "SYNC_SERVICE_HOST"
	envVarSyncServicePort     = "SYNC_SERVICE_PORT"
)

var (
	errEnvVarNotFound  = errors.New("not found environment variable")
	errEnvVarWrongType = errors.New("wrong type of environment variable")
)

// SyncService abstracts Sync Service client.
type SyncService struct {
	client    *client.SyncServiceClient
	msgChan   chan *syncServiceMessage
	stopChan  chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	log       logr.Logger
}

// NewSyncService creates a new instance of SyncService.
func NewSyncService(log logr.Logger) (*SyncService, error) {
	serverProtocol, host, port, err := readEnvVars()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize sync service - %w", err)
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

func readEnvVars() (string, string, uint16, error) {
	protocol := os.Getenv(envVarSyncServiceProtocol)
	if protocol == "" {
		return "", "", 0, fmt.Errorf("%w: %s", errEnvVarNotFound, envVarSyncServiceProtocol)
	}

	host := os.Getenv(envVarSyncServiceHost)
	if host == "" {
		return "", "", 0, fmt.Errorf("%w: %s", errEnvVarNotFound, envVarSyncServiceHost)
	}

	portStr := os.Getenv(envVarSyncServicePort)
	if portStr == "" {
		return "", "", 0, fmt.Errorf("%w: %s", errEnvVarNotFound, envVarSyncServicePort)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", "", 0, fmt.Errorf("%w: %s must be an integer", errEnvVarWrongType, envVarSyncServicePort)
	}

	return protocol, host, uint16(port), nil
}

// Start function starts sync service.
func (s *SyncService) Start() {
	s.startOnce.Do(func() {
		go s.sendMessages()
	})
}

// Stop function stops sync service.
func (s *SyncService) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopChan)
	})
}

// SendAsync function sends a message to the sync service asynchronously.
func (s *SyncService) SendAsync(id string, msgType string, version string, payload []byte) {
	message := &syncServiceMessage{
		id:      id,
		msgType: msgType,
		version: version,
		payload: payload,
	}
	s.msgChan <- message
}

// GetVersion if the object doesn't exist or an error occurred returns an empty string, otherwise returns the version.
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

			if err := s.client.UpdateObject(&metaData); err != nil {
				s.log.Error(err, "Failed to update the object in the Edge Sync Service")
				continue
			}

			reader := bytes.NewReader(msg.payload)
			if err := s.client.UpdateObjectData(&metaData, reader); err != nil {
				s.log.Error(err, "Failed to update the object data in the Edge Sync Service")
				continue
			}

			s.log.Info(fmt.Sprintf("Message '%s' from type '%s' with version '%s' sent", msg.id, msg.msgType,
				msg.version))
		}
	}
}
