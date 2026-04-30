package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// Producer wraps a kafka-go writer.
type Producer struct {
	writers map[string]*kafkago.Writer
	brokers []string
}

// NewProducer creates a new Kafka producer.
func NewProducer() (*Producer, error) {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9092"
	}
	return &Producer{
		writers: make(map[string]*kafkago.Writer),
		brokers: strings.Split(brokers, ","),
	}, nil
}

func (pr *Producer) writerFor(topic string) *kafkago.Writer {
	if w, ok := pr.writers[topic]; ok {
		return w
	}
	w := &kafkago.Writer{
		Addr:                   kafkago.TCP(pr.brokers...),
		Topic:                  topic,
		Balancer:               &kafkago.LeastBytes{},
		MaxAttempts:            5,
		WriteTimeout:           10 * time.Second,
		RequiredAcks:           kafkago.RequireAll,
		AllowAutoTopicCreation: true,
	}
	pr.writers[topic] = w
	return w
}

// Produce sends a message to a Kafka topic (fire-and-forget async).
func (pr *Producer) Produce(topic, key string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = pr.writerFor(topic).WriteMessages(ctx, kafkago.Message{
		Key:   []byte(key),
		Value: data,
	})
	if err != nil {
		log.Printf("kafka produce error topic=%s: %v", topic, err)
	}
	return err
}

// ProduceSync sends a message and waits for delivery (for GameOver EOS guarantee).
func (pr *Producer) ProduceSync(topic, key string, payload interface{}) error {
	return pr.Produce(topic, key, payload)
}

// Flush is a no-op for kafka-go (writes are synchronous per message).
func (pr *Producer) Flush(_ int) {}

// Close shuts down all writers.
func (pr *Producer) Close() {
	for _, w := range pr.writers {
		w.Close()
	}
}
