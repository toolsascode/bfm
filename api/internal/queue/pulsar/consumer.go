package pulsar

import (
	"context"
	"encoding/json"
	"fmt"

	"bfm/api/internal/logger"
	"bfm/api/internal/queue"

	"github.com/apache/pulsar-client-go/pulsar"
)

// Consumer implements queue.Consumer using Pulsar
type Consumer struct {
	client   pulsar.Client
	consumer pulsar.Consumer
	topic    string
}

// NewConsumer creates a new Pulsar consumer
func NewConsumer(url, topic, subscriptionName string) (*Consumer, error) {
	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL: url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Pulsar client: %w", err)
	}

	consumer, err := client.Subscribe(pulsar.ConsumerOptions{
		Topic:            topic,
		SubscriptionName: subscriptionName,
		Type:             pulsar.Shared,
	})
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create Pulsar consumer: %w", err)
	}

	return &Consumer{
		client:   client,
		consumer: consumer,
		topic:    topic,
	}, nil
}

// Consume starts consuming jobs from Pulsar
func (c *Consumer) Consume(ctx context.Context, handler queue.JobHandler) error {
	logger.Infof("Starting Pulsar consumer for topic %s", c.topic)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Pulsar consumer context cancelled")
			return ctx.Err()
		default:
			// Receive message from Pulsar
			msg, err := c.consumer.Receive(ctx)
			if err != nil {
				return fmt.Errorf("failed to receive message from Pulsar: %w", err)
			}

			// Deserialize job
			var job queue.Job
			if err := json.Unmarshal(msg.Payload(), &job); err != nil {
				logger.Errorf("Failed to unmarshal job from Pulsar message: %v", err)
				// Acknowledge and continue processing other messages
				c.consumer.Ack(msg)
				continue
			}

			// Extract job ID from properties if not in body
			if job.ID == "" {
				if jobID, ok := msg.Properties()["job-id"]; ok {
					job.ID = jobID
				} else if msg.Key() != "" {
					job.ID = msg.Key()
				}
			}

			logger.Infof("Processing migration job %s from Pulsar", job.ID)

			// Process job
			result, err := handler(ctx, &job)
			if err != nil {
				logger.Errorf("Failed to process migration job %s: %v", job.ID, err)
				// Negative acknowledge to retry later
				c.consumer.Nack(msg)
				continue
			}

			// Acknowledge message
			if err := c.consumer.Ack(msg); err != nil {
				logger.Errorf("Failed to acknowledge message for job %s: %v", job.ID, err)
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

// Close closes the Pulsar consumer
func (c *Consumer) Close() error {
	c.consumer.Close()
	c.client.Close()
	return nil
}
