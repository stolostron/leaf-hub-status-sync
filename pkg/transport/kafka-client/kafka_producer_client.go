package kafkaclient

import (
	"encoding/json"
	"fmt"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"
	kclient "github.com/open-cluster-management/hub-of-hubs-kafka-transport/kafka-client/kafka-producer"
	"github.com/open-cluster-management/leaf-hub-status-sync/pkg/transport"
)

// NewLHProducer returns a new instance of LHProducer object.
func NewLHProducer(log logr.Logger) (*LHProducer, error) {
	kp := &LHProducer{
		deliveryChan:  make(chan kafka.Event),
		stopChan:      make(chan struct{}, 1),
		kafkaProducer: nil,
		log:           log,
	}

	kafkaProducer, err := kclient.NewKafkaProducer(kp.deliveryChan)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka producer: %w", err)
	}

	kp.kafkaProducer = kafkaProducer

	return kp, nil
}

// LHProducer abstracts hub-of-hubs-kafka-transport kafka-producer's generic usage.
type LHProducer struct {
	kafkaProducer *kclient.KafkaProducer
	deliveryChan  chan kafka.Event
	stopChan      chan struct{}
	log           logr.Logger
}

// deliveryHandler handles results of sent messages.
// For now failed messages are only logged.
func (p *LHProducer) deliveryHandler(e *kafka.Event) {
	switch ev := (*e).(type) {
	case *kafka.Message:
		if ev.TopicPartition.Error != nil {
			load := &transport.Message{}

			err := json.Unmarshal(ev.Value, load)
			if err != nil {
				p.log.Error(err, "Failed to deliver message",
					"Topic Name", ev.TopicPartition)
				return
			}

			p.log.Error(ev.TopicPartition.Error, "Failed to deliver message",
				"Message ID", load.ID, "Topic Name", ev.TopicPartition)
		}
	default:
		p.log.Info("Received unsupported kafka-event type", "Message Type", ev)
	}
}

// Start starts the kafka-client.
func (p *LHProducer) Start() {
	// Delivery report handler for produced messages
	go func() {
		for {
			select {
			case <-p.stopChan:
				return
			case e := <-p.deliveryChan:
				p.deliveryHandler(&e)
			}
		}
	}()
}

// Stop stops the kafka-client.
func (p *LHProducer) Stop() {
	p.kafkaProducer.Close()
	p.stopChan <- struct{}{}
	close(p.stopChan)
	close(p.deliveryChan)
}

// SendAsync sends a message to the sync service asynchronously.
func (p *LHProducer) SendAsync(id string, msgType string, version string, payload []byte) {
	message := &transport.Message{
		ID:      id,
		MsgType: msgType,
		Version: version,
		Payload: payload,
	}

	bs, err := json.Marshal(message)
	if err != nil {
		p.log.Error(err, "Failed to send message", "Message ID", message.ID)
		return
	}

	err = p.kafkaProducer.ProduceAsync(&bs)
	if err != nil {
		p.log.Error(err, "Failed to send message", "Message ID", message.ID)
	}
}

// GetVersion returns an empty string if the object doesn't exist or an error occurred.
func (p *LHProducer) GetVersion(id string, msgType string) string {
	// TODO: implement with consumer
	return ""
}
