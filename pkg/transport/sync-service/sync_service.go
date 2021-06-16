package sync_service

import (
	"bytes"
	"github.com/open-horizon/edge-sync-service-client/client"
	"log"
	"os"
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
	msgChan   chan *syncServiceMessage
	stopChan  chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewSyncService() *SyncService {
	serverProtocol, host, port := readEnvVars()
	syncServiceClient := client.NewSyncServiceClient(serverProtocol, host, port)
	syncServiceClient.SetAppKeyAndSecret("user@myorg", "")
	return &SyncService{
		client:   syncServiceClient,
		msgChan:  make(chan *syncServiceMessage),
		stopChan: make(chan struct{}, 1),
	}
}

func readEnvVars() (string, string, uint16) {
	protocol := os.Getenv(syncServiceProtocol)
	if protocol == "" {
		log.Fatalf("the expected var %s is not set in environment variables", syncServiceProtocol)
	}
	host := os.Getenv(syncServiceHost)
	if host == "" {
		log.Fatalf("the expected var %s is not set in environment variables", syncServiceHost)
	}
	portStr := os.Getenv(syncServicePort)
	if portStr == "" {
		log.Fatalf("the expected env var %s is not set in environment variables", syncServicePort)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Fatalf("the expected env var %s is not from type int", syncServicePort)
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
				log.Printf("Failed to update the object in the Edge Sync Service. Error: %s\n", err)
				continue
			}
			reader := bytes.NewReader(msg.payload)
			err = s.client.UpdateObjectData(&metaData, reader)
			if err != nil {
				log.Printf("Failed to update the object data in the Edge Sync Service. Error: %s\n", err)
				continue
			}
			log.Printf("Message '%s' from type '%s' with version '%s' sent", msg.id, msg.msgType, msg.version)
		}
	}
}
