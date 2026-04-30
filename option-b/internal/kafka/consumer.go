package kafka

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"github.com/rotr/ring-of-the-middle-earth/internal/router"
)

// Consumer wraps kafka-go readers per topic.
type Consumer struct {
	readers []*kafkago.Reader
	groupID string
}

// NewConsumer creates a Kafka consumer in the given group.
func NewConsumer(groupID string) (*Consumer, error) {
	return &Consumer{groupID: groupID}, nil
}

// Subscribe creates readers for the given topics.
func (c *Consumer) Subscribe(topics []string) error {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		brokers = "localhost:9092"
	}

	for _, topic := range topics {
		r := kafkago.NewReader(kafkago.ReaderConfig{
			Brokers:        strings.Split(brokers, ","),
			Topic:          topic,
			GroupID:        c.groupID,
			MinBytes:       1,
			MaxBytes:       10e6,
			MaxWait:        100 * time.Millisecond,
			CommitInterval: time.Second,
		})
		c.readers = append(c.readers, r)
	}
	return nil
}

// ConsumeLoop reads messages from all subscribed topics and sends them to eventCh.
func (c *Consumer) ConsumeLoop(ctx context.Context, eventCh chan<- router.Event) {
	defer func() {
		for _, r := range c.readers {
			r.Close()
		}
	}()

	// Start one goroutine per reader
	for _, reader := range c.readers {
		r := reader
		go func() {
			for {
				msg, err := r.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("Consumer read error topic=%s: %v", r.Config().Topic, err)
					continue
				}

				var payload interface{}
				if err := json.Unmarshal(msg.Value, &payload); err != nil {
					log.Printf("Failed to unmarshal message: %v", err)
					continue
				}

				select {
				case eventCh <- router.Event{
					Topic:   r.Config().Topic,
					Key:     string(msg.Key),
					Payload: payload,
				}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	<-ctx.Done()
}
