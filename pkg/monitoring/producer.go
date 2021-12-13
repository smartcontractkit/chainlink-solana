package monitoring

import (
	"context"
	"fmt"
	"log"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

// Producer is an abstraction on top of Kafka to aid with tests.
type Producer interface {
	Produce(key, value []byte) error
}

type producer struct {
	backend      *kafka.Producer
	deliveryChan chan kafka.Event
	topic        string
}

func NewProducer(ctx context.Context, cfg KafkaConfig, topic string) (Producer, error) {
	backend, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": cfg.Brokers,
		"client.id":         cfg.ClientID,
		"security.protocol": cfg.SecurityProtocol,
		"sasl.mechanisms":   cfg.SaslMechanism,
		"sasl.username":     cfg.SaslUsername,
		"sasl.password":     cfg.SaslPassword,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka envelopeProducer: %w", err)
	}
	p := &producer{
		backend,
		make(chan kafka.Event),
		topic,
	}
	go p.run(ctx)
	return p, nil
}

// run should be executed as a goroutine.
func (p *producer) run(ctx context.Context) {
	for {
		select {
		case event := <-p.deliveryChan:
			log.Printf("received response event for a message delivery: %s", event.String())
		case <-ctx.Done():
			p.backend.Close()
			return
		}
	}
}

func (p *producer) Produce(key, value []byte) error {
	return p.backend.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &p.topic,
			Partition: kafka.PartitionAny,
		},
		Key:   key,
		Value: value,
	}, p.deliveryChan)
}
