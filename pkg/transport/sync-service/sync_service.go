package syncservice

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/go-logr/logr"
	"github.com/open-cluster-management/hub-of-hubs-message-compression/compressors"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
	"github.com/open-horizon/edge-sync-service-client/client"
)

const (
	envVarSyncServiceProtocol = "SYNC_SERVICE_PROTOCOL"
	envVarSyncServiceHost     = "SYNC_SERVICE_HOST"
	envVarSyncServicePort     = "SYNC_SERVICE_PORT"
	compressionHeader         = "Content-Encoding"
)

var (
	errEnvVarNotFound    = errors.New("environment variable not found")
	errEnvVarInvalidType = errors.New("environment variable invalid type")
)

// NewSyncService creates a new instance of SyncService.
func NewSyncService(compressor compressors.Compressor, log logr.Logger) (*SyncService, error) {
	serverProtocol, host, port, err := readEnvVars()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize sync service - %w", err)
	}

	syncServiceClient := client.NewSyncServiceClient(serverProtocol, host, port)

	syncServiceClient.SetAppKeyAndSecret("user@myorg", "")

	return &SyncService{
		log:        log,
		client:     syncServiceClient,
		compressor: compressor,
		msgChan:    make(chan *transport.Message),
		stopChan:   make(chan struct{}, 1),
	}, nil
}

func readEnvVars() (string, string, uint16, error) {
	protocol, found := os.LookupEnv(envVarSyncServiceProtocol)
	if !found {
		return "", "", 0, fmt.Errorf("%w: %s", errEnvVarNotFound, envVarSyncServiceProtocol)
	}

	host, found := os.LookupEnv(envVarSyncServiceHost)
	if !found {
		return "", "", 0, fmt.Errorf("%w: %s", errEnvVarNotFound, envVarSyncServiceHost)
	}

	portStr, found := os.LookupEnv(envVarSyncServicePort)
	if !found {
		return "", "", 0, fmt.Errorf("%w: %s", errEnvVarNotFound, envVarSyncServicePort)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", "", 0, fmt.Errorf("%w: %s must be an integer", errEnvVarInvalidType, envVarSyncServicePort)
	}

	return protocol, host, uint16(port), nil
}

// SyncService abstracts Sync Service client.
type SyncService struct {
	log        logr.Logger
	client     *client.SyncServiceClient
	compressor compressors.Compressor
	msgChan    chan *transport.Message
	stopChan   chan struct{}
	startOnce  sync.Once
	stopOnce   sync.Once
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
		s.stopChan <- struct{}{}
		close(s.stopChan)
		close(s.msgChan)
	})
}

// SendAsync function sends a message to the sync service asynchronously.
func (s *SyncService) SendAsync(message *transport.Message) {
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
			objectMetaData := client.ObjectMetaData{
				ObjectID:    msg.ID,
				ObjectType:  msg.MsgType,
				Version:     msg.Version,
				Description: fmt.Sprintf("%s:%s", compressionHeader, s.compressor.GetType()),
			}

			if err := s.client.UpdateObject(&objectMetaData); err != nil {
				s.log.Error(err, "Failed to update the object in the Edge Sync Service")
				continue
			}

			compressedBytes, err := s.compressor.Compress(msg.Payload)
			if err != nil {
				s.log.Error(err, "Failed to compress payload", "CompressorType",
					s.compressor.GetType(), "MessageId", msg.ID, "MessageType", msg.MsgType, "Version",
					msg.Version)

				continue
			}

			reader := bytes.NewReader(compressedBytes)
			if err := s.client.UpdateObjectData(&objectMetaData, reader); err != nil {
				s.log.Error(err, "Failed to update the object data in the Edge Sync Service")
				continue
			}

			s.log.Info("Message sent successfully", "MessageId", msg.ID, "MessageType", msg.MsgType,
				"Version", msg.Version)
		}
	}
}
