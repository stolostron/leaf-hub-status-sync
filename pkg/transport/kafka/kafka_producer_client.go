package kafka

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"
	kafkaClient "github.com/open-cluster-management/hub-of-hubs-kafka-transport/kafka-client/kafka-producer"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

const bufferedChannelSize = 500

// NewProducer returns a new instance of Producer object.
func NewProducer(log logr.Logger) (*Producer, error) {
	deliveryChan := make(chan kafka.Event, bufferedChannelSize)

	kafkaProducer, err := kafkaClient.NewKafkaProducer(deliveryChan)
	if err != nil {
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	return &Producer{
		log:           log,
		kafkaProducer: kafkaProducer,
		deliveryChan:  deliveryChan,
		stopChan:      make(chan struct{}),
	}, nil
}

// Producer abstracts hub-of-hubs-kafka-transport kafka-producer's generic usage.
type Producer struct {
	log           logr.Logger
	kafkaProducer *kafkaClient.KafkaProducer
	deliveryChan  chan kafka.Event

	stopChan  chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

// deliveryHandler handles results of sent messages.
// For now failed messages are only logged.
func (p *Producer) deliveryHandler(kafkaEvent *kafka.Event) {
	switch event := (*kafkaEvent).(type) {
	case *kafka.Message:
		if event.TopicPartition.Error != nil {
			message := &transport.Message{}

			if err := json.Unmarshal(event.Value, message); err != nil {
				p.log.Error(err, "failed to deliver message", "topic", event.TopicPartition)
				return
			}

			p.log.Error(event.TopicPartition.Error, "failed to deliver message",
				"message id", message.ID, "topic", event.TopicPartition)
		}
	default:
		p.log.Info("received unsupported kafka-event type", "event type", event)
	}
}

// Start starts the kafka.
func (p *Producer) Start() {
	p.startOnce.Do(func() {
		go p.handleDelivery()
	})
}

// Stop stops the producer.
func (p *Producer) Stop() {
	p.stopOnce.Do(func() {
		p.kafkaProducer.Close()
		close(p.deliveryChan)
		close(p.stopChan)
	})
}

// SendAsync sends a message to the sync service asynchronously.
func (p *Producer) SendAsync(id string, msgType string, version string, payload []byte) {
	message := &transport.Message{
		ID:      id,
		MsgType: msgType,
		Version: version,
		Payload: payload,
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		p.log.Error(err, "failed to send message", "message id", message.ID)
		return
	}

	if err = p.kafkaProducer.ProduceAsync(messageBytes, []byte(id), []byte(msgType), []byte(version)); err != nil {
		p.log.Error(err, "failed to send message", "message id", message.ID)
	}
}

// GetVersion returns an empty string if the object doesn't exist or an error occurred.
func (p *Producer) GetVersion(id string, msgType string) string {
	return ""
}

func (p *Producer) handleDelivery() {
	for {
		select {
		case <-p.stopChan:
			return

		case event := <-p.deliveryChan:
			p.deliveryHandler(&event)
		}
	}
}
