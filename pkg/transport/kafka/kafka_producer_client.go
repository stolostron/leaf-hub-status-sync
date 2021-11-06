package kafka

import (
	"encoding/json"
	"fmt"
	kafkaHeaderTypes "github.com/open-cluster-management/hub-of-hubs-kafka-transport/types"
	"sync"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"
	kafkaClient "github.com/open-cluster-management/hub-of-hubs-kafka-transport/kafka-client/kafka-producer"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

const (
	bufferedChannelSize = 500
)

// NewProducer returns a new instance of Producer object.
func NewProducer(log logr.Logger) (*Producer, error) {
	deliveryChan := make(chan kafka.Event, bufferedChannelSize)

	kafkaProducer, err := kafkaClient.NewKafkaProducer(deliveryChan)
	if err != nil {
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	return &Producer{
		log:                     log,
		kafkaProducer:           kafkaProducer,
		deliveryChan:            deliveryChan,
		deliveryRegistrationMap: make(map[string]*transport.BundleDeliveryRegistration),
		stopChan:                make(chan struct{}),
	}, nil
}

// Producer abstracts hub-of-hubs-kafka-transport kafka-producer's generic usage.
type Producer struct {
	log                     logr.Logger
	kafkaProducer           *kafkaClient.KafkaProducer
	deliveryChan            chan kafka.Event
	deliveryRegistrationMap map[string]*transport.BundleDeliveryRegistration
	stopChan                chan struct{}
	startOnce               sync.Once
	stopOnce                sync.Once
}

// deliveryHandler handles results of sent messages.
// For now failed messages are only logged.
func (p *Producer) deliveryHandler(kafkaMessage *kafka.Message) {
	// TODO (maybe): take out logic into a separate controller
	transportMessage := &transport.Message{}
	if err := json.Unmarshal(kafkaMessage.Value, transportMessage); err != nil {
		return
	}

	if kafkaMessage.TopicPartition.Error != nil {
		p.handleFailure(transportMessage, &kafkaMessage.TopicPartition.Error)

		return
	}

	// invoke success event for registered handler if found
	if deliveryRegistration, found := p.deliveryRegistrationMap[transportMessage.ID]; found {
		go func() {
			deliveryRegistration.InvokeEventActions(transport.DeliverySuccess, transport.ArgTypeNone, nil)
		}()
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
func (p *Producer) SendAsync(message *transport.Message) {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		p.log.Error(err, "failed to send message", "message id", message.ID)
		return
	}

	headers := []kafka.Header{
		{Key: kafkaHeaderTypes.MsgIDKey, Value: []byte(message.ID)},
		{Key: kafkaHeaderTypes.MsgTypeKey, Value: []byte(message.MsgType)},
	}
	if err = p.kafkaProducer.ProduceAsync(messageBytes, headers); err != nil {
		p.handleFailure(message, &err)
	}

	p.log.Info("sent bundle", "ID", message.ID, "version", message.Version, "type", message.MsgType)
}

// Register maps type to bundle delivery registration info.
func (p *Producer) Register(msgType string, deliveryRegistration *transport.BundleDeliveryRegistration) {
	p.deliveryRegistrationMap[msgType] = deliveryRegistration
}

func (p *Producer) handleDelivery() {
	for {
		select {
		case <-p.stopChan:
			return

		case event := <-p.deliveryChan:
			kafkaMessage, ok := event.(*kafka.Message)
			if !ok {
				p.log.Info("received unsupported kafka-event type", "event type", event)
			} else {
				p.deliveryHandler(kafkaMessage)
			}
		}
	}
}

func (p *Producer) handleFailure(transportMessage *transport.Message, err *error) {
	// alert registered channels
	if deliveryRegistration, found := p.deliveryRegistrationMap[transportMessage.ID]; found {
		go func() {
			// let relevant handlers know of this failure
			deliveryRegistration.PropagateFailure(err)
			// let relevant handlers schedule retry (if enabled)
			deliveryRegistration.PropagateRetry(transportMessage)
		}()
	}

	p.log.Error(*err, "failed to deliver message",
		"bundle id", transportMessage.ID, "bundle type", transportMessage.MsgType,
		"bundle version", transportMessage.Version)
}
