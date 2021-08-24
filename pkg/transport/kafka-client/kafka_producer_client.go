package kafkaclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"
	kclient "github.com/open-cluster-management/hub-of-hubs-kafka-transport/kafka-client/kafka-producer"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

// NewProducer returns a new instance of Producer object.
func NewProducer(log logr.Logger) (*Producer, error) {
	kp := &Producer{
		log:           log,
		kafkaProducer: nil,
		deliveryChan:  make(chan kafka.Event),
	}

	kafkaProducer, err := kclient.NewKafkaProducer(kp.deliveryChan)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka producer: %w", err)
	}

	kp.kafkaProducer = kafkaProducer

	return kp, nil
}

// Producer abstracts hub-of-hubs-kafka-transport kafka-producer's generic usage.
type Producer struct {
	log           logr.Logger
	kafkaProducer *kclient.KafkaProducer
	deliveryChan  chan kafka.Event
}

// deliveryHandler handles results of sent messages.
// For now failed messages are only logged.
func (p *Producer) deliveryHandler(kafkaEvent *kafka.Event) {
	switch event := (*kafkaEvent).(type) {
	case *kafka.Message:
		if event.TopicPartition.Error != nil {
			load := &transport.Message{}

			err := json.Unmarshal(event.Value, load)
			if err != nil {
				p.log.Error(err, "Failed to deliver message",
					"topic name", event.TopicPartition)
				return
			}

			p.log.Error(event.TopicPartition.Error, "Failed to deliver message",
				"message id", load.ID, "topic name", event.TopicPartition)
		}
	default:
		p.log.Info("Received unsupported kafka-event type", "Message Type", event)
	}
}

// Start starts the kafka-client.
func (p *Producer) Start(stopChannel <-chan struct{}) error {
	ctx, cancelContext := context.WithCancel(context.Background())
	defer cancelContext()

	go p.handleDelivery(ctx)

	for {
		<-stopChannel // blocking wait until getting stop event on the stop channel.
		cancelContext()

		p.kafkaProducer.Close()
		close(p.deliveryChan)

		p.log.Info("stopped kafka producer")

		return nil
	}
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
		p.log.Error(err, "Failed to send message", "Message ID", message.ID)
		return
	}

	if err = p.kafkaProducer.ProduceAsync(messageBytes, []byte(id), []byte(msgType), []byte(version)); err != nil {
		p.log.Error(err, "Failed to send message", "Message ID", message.ID)
	}
}

// GetVersion returns an empty string if the object doesn't exist or an error occurred.
func (p *Producer) GetVersion(id string, msgType string) string {
	// we know that the listening transport bridge does not care about generations due to message committing design.
	return "0"
}

func (p *Producer) handleDelivery(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case e := <-p.deliveryChan:
			p.deliveryHandler(&e)
		}
	}
}
