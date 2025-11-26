package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/toolsascode/bfm/api/internal/logger"
	"github.com/toolsascode/bfm/api/internal/queue"

	"github.com/segmentio/kafka-go"
)

// Consumer implements queue.Consumer using Kafka
type Consumer struct {
	reader *kafka.Reader
	topic  string
}

// NewConsumer creates a new Kafka consumer
func NewConsumer(brokers []string, topic, groupID string) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
	})

	return &Consumer{
		reader: reader,
		topic:  topic,
	}
}

// Consume starts consuming jobs from Kafka
func (c *Consumer) Consume(ctx context.Context, handler queue.JobHandler) error {
	logger.Infof("Starting Kafka consumer for topic %s", c.topic)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Kafka consumer context cancelled")
			return ctx.Err()
		default:
			// Read message from Kafka
			msg, err := c.reader.ReadMessage(ctx)
			if err != nil {
				return fmt.Errorf("failed to read message from Kafka: %w", err)
			}

			// Deserialize job
			var job queue.Job
			if err := json.Unmarshal(msg.Value, &job); err != nil {
				logger.Errorf("Failed to unmarshal job from Kafka message: %v", err)
				// Continue processing other messages
				continue
			}

			// Extract job ID from headers if not in body
			if job.ID == "" {
				for _, header := range msg.Headers {
					if header.Key == "job-id" {
						job.ID = string(header.Value)
						break
					}
				}
			}

			logger.Infof("Processing migration job %s from Kafka", job.ID)

			// Process job
			result, err := handler(ctx, &job)
			if err != nil {
				logger.Errorf("Failed to process migration job %s: %v", job.ID, err)
				// Continue processing other messages
				continue
			}

			if result != nil {
				if result.Success {
					logger.Infof("Successfully processed migration job %s: %d applied, %d skipped",
						job.ID, len(result.Applied), len(result.Skipped))
				} else {
					logger.Warnf("Migration job %s completed with errors: %v", job.ID, result.Errors)
				}
			}
		}
	}
}

// Close closes the Kafka consumer
func (c *Consumer) Close() error {
	return c.reader.Close()
}
