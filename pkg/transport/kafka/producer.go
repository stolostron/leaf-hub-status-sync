package kafka

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"
	"github.com/open-cluster-management/hub-of-hubs-kafka-transport/headers"
	kafkaclient "github.com/open-cluster-management/hub-of-hubs-kafka-transport/kafka-client"
	kafkaproducer "github.com/open-cluster-management/hub-of-hubs-kafka-transport/kafka-client/kafka-producer"
	"github.com/open-cluster-management/hub-of-hubs-message-compression/compressors"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

const (
	envVarKafkaProducerID       = "KAFKA_PRODUCER_ID"
	envVarKafkaBootstrapServers = "KAFKA_BOOTSTRAP_SERVERS"
	envVarKafkaTopic            = "KAFKA_TOPIC"
	envVarKafkaSSLCA            = "KAFKA_SSL_CA"
	envVarMessageSizeLimit      = "KAFKA_MESSAGE_SIZE_LIMIT_KB"

	maxMessageSizeLimit = 987 // to make sure that the message size is below 1 MB.
	partition           = 0
	kiloBytesToBytes    = 1000
)

var (
	errEnvVarNotFound     = errors.New("environment variable not found")
	errEnvVarIllegalValue = errors.New("environment variable illegal value")
)

// NewProducer returns a new instance of Producer object.
func NewProducer(compressor compressors.Compressor, log logr.Logger) (*Producer, error) {
	deliveryChan := make(chan kafka.Event)

	kafkaConfigMap, topic, messageSizeLimit, err := readEnvVars()
	if err != nil {
		close(deliveryChan)
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	kafkaProducer, err := kafkaproducer.NewKafkaProducer(kafkaConfigMap, messageSizeLimit*kiloBytesToBytes,
		deliveryChan)
	if err != nil {
		close(deliveryChan)
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	return &Producer{
		log:                  log,
		kafkaProducer:        kafkaProducer,
		topic:                topic,
		eventSubscriptionMap: make(map[string]map[transport.EventType]transport.EventCallback),
		compressor:           compressor,
		deliveryChan:         deliveryChan,
		stopChan:             make(chan struct{}),
	}, nil
}

func readEnvVars() (*kafka.ConfigMap, string, int, error) {
	producerID, found := os.LookupEnv(envVarKafkaProducerID)
	if !found {
		return nil, "", 0, fmt.Errorf("%w: %s", errEnvVarNotFound, envVarKafkaProducerID)
	}

	bootstrapServers, found := os.LookupEnv(envVarKafkaBootstrapServers)
	if !found {
		return nil, "", 0, fmt.Errorf("%w: %s", errEnvVarNotFound, envVarKafkaBootstrapServers)
	}

	topic, found := os.LookupEnv(envVarKafkaTopic)
	if !found {
		return nil, "", 0, fmt.Errorf("%w: %s", errEnvVarNotFound, envVarKafkaTopic)
	}

	messageSizeLimitString, found := os.LookupEnv(envVarMessageSizeLimit)
	if !found {
		return nil, "", 0, fmt.Errorf("%w: %s", errEnvVarNotFound, envVarMessageSizeLimit)
	}

	messageSizeLimit, err := strconv.Atoi(messageSizeLimitString)
	if err != nil || messageSizeLimit <= 0 {
		return nil, "", 0, fmt.Errorf("%w: %s", errEnvVarIllegalValue, envVarMessageSizeLimit)
	}

	if messageSizeLimit > maxMessageSizeLimit {
		return nil, "", 0, fmt.Errorf("%w - size must not exceed %d : %s", errEnvVarIllegalValue,
			maxMessageSizeLimit, envVarMessageSizeLimit)
	}

	kafkaConfigMap := &kafka.ConfigMap{
		"bootstrap.servers": bootstrapServers,
		"client.id":         producerID,
		"acks":              "1",
		"retries":           "0",
	}

	if err := readSSLEnvVar(kafkaConfigMap); err != nil {
		return nil, "", 0, fmt.Errorf("%w - failed to read SSL env var", err)
	}

	return kafkaConfigMap, topic, messageSizeLimit, nil
}

func readSSLEnvVar(kafkaConfigMap *kafka.ConfigMap) error {
	if sslBase64EncodedCertificate, found := os.LookupEnv(envVarKafkaSSLCA); found {
		certFileLocation, err := kafkaclient.SetCertificate(&sslBase64EncodedCertificate)
		if err != nil {
			return fmt.Errorf("failed to configure kafka-producer - %w", err)
		}

		if err = kafkaConfigMap.SetKey("security.protocol", "ssl"); err != nil {
			return fmt.Errorf("failed to configure kafka-producer - %w", err)
		}

		if err = kafkaConfigMap.SetKey("ssl.ca.location", certFileLocation); err != nil {
			return fmt.Errorf("failed to configure kafka-producer - %w", err)
		}
	}

	return nil
}

// Producer abstracts hub-of-hubs-kafka-transport kafka-producer's generic usage.
type Producer struct {
	log                  logr.Logger
	kafkaProducer        *kafkaproducer.KafkaProducer
	eventSubscriptionMap map[string]map[transport.EventType]transport.EventCallback
	topic                string
	compressor           compressors.Compressor
	deliveryChan         chan kafka.Event
	stopChan             chan struct{}
	startOnce            sync.Once
	stopOnce             sync.Once
}

// Start starts the kafka.
func (p *Producer) Start() {
	p.startOnce.Do(func() {
		go p.deliveryReportHandler()
	})
}

// Stop stops the producer.
func (p *Producer) Stop() {
	p.stopOnce.Do(func() {
		p.stopChan <- struct{}{}
		close(p.deliveryChan)
		close(p.stopChan)
		p.kafkaProducer.Close()
	})
}

func (p *Producer) deliveryReportHandler() {
	for {
		select {
		case <-p.stopChan:
			return

		case event := <-p.deliveryChan:
			kafkaMessage, ok := event.(*kafka.Message)
			if !ok {
				p.log.Info("received unsupported kafka-event type", "event type", event)
				continue
			}

			p.handleDeliveryReport(kafkaMessage)
		}
	}
}

// handleDeliveryReport handles results of sent messages.
func (p *Producer) handleDeliveryReport(kafkaMessage *kafka.Message) {
	if kafkaMessage.TopicPartition.Error != nil {
		p.log.Error(kafkaMessage.TopicPartition.Error, "failed to deliver message",
			"MessageId", string(kafkaMessage.Key), "TopicPartition", kafkaMessage.TopicPartition)
		transport.InvokeCallback(p.eventSubscriptionMap, string(kafkaMessage.Key), transport.DeliveryFailure)

		return
	}

	transport.InvokeCallback(p.eventSubscriptionMap, string(kafkaMessage.Key), transport.DeliverySuccess)
}

// Subscribe adds a callback to be delegated when a given event occurs for a message with the given ID.
func (p *Producer) Subscribe(messageID string, callbacks map[transport.EventType]transport.EventCallback) {
	p.eventSubscriptionMap[messageID] = callbacks
}

// SupportsDeltaBundles returns true. kafka does support delta bundles.
func (p *Producer) SupportsDeltaBundles() bool {
	return true
}

// SendAsync sends a message to the sync service asynchronously.
func (p *Producer) SendAsync(message *transport.Message) {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		p.log.Error(err, "failed to send message", "MessageId", message.ID, "MessageType",
			message.MsgType, "Version", message.Version)

		return
	}

	compressedBytes, err := p.compressor.Compress(messageBytes)
	if err != nil {
		p.log.Error(err, "failed to compress bundle", "CompressorType", p.compressor.GetType(),
			"MessageId", message.ID, "MessageType", message.MsgType, "Version", message.Version)

		return
	}

	messageHeaders := []kafka.Header{
		{Key: headers.CompressionType, Value: []byte(p.compressor.GetType())},
	}

	if err = p.kafkaProducer.ProduceAsync(message.Key, p.topic, partition, headers, compressedBytes); err != nil {
		p.log.Error(err, "failed to send message", "MessageKey", message.Key, "MessageId", message.ID,
			"MessageType", message.MsgType, "Version", message.Version)
		transport.InvokeCallback(p.eventSubscriptionMap, message.ID, transport.DeliveryFailure)

		return
	}

	transport.InvokeCallback(p.eventSubscriptionMap, message.ID, transport.DeliveryAttempt)

	p.log.Info("message sent to transport server", "MessageKey", message.Key, "MessageId", message.ID,
		"MessageType", message.MsgType, "Version", message.Version)
}
