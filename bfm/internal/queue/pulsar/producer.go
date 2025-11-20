package pulsar

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"mops/bfm/internal/logger"
	"mops/bfm/internal/queue"
)

// Producer implements queue.Producer using Pulsar
type Producer struct {
	client pulsar.Client
	producer pulsar.Producer
	topic  string
}

// NewProducer creates a new Pulsar producer
func NewProducer(url, topic string) (*Producer, error) {
	client, err := pulsar.NewClient(pulsar.ClientOptions{
		URL: url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Pulsar client: %w", err)
	}

	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic: topic,
	})
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create Pulsar producer: %w", err)
	}

	return &Producer{
		client:   client,
		producer: producer,
		topic:    topic,
	}, nil
}

// PublishJob publishes a migration job to Pulsar
func (p *Producer) PublishJob(ctx context.Context, job *queue.Job) error {
	// Generate job ID if not provided
	if job.ID == "" {
		job.ID = fmt.Sprintf("job_%d", time.Now().UnixNano())
	}

	// Serialize job to JSON
	jobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	// Create Pulsar message
	msg := &pulsar.ProducerMessage{
		Payload: jobData,
		Key:     job.ID,
		Properties: map[string]string{
			"job-id":     job.ID,
			"connection": job.Connection,
		},
	}

	// Publish message
	_, err = p.producer.Send(ctx, msg)
	if err != nil {
		return fmt.Errorf("failed to send message to Pulsar: %w", err)
	}

	logger.Infof("Published migration job %s to Pulsar topic %s", job.ID, p.topic)
	return nil
}

// Close closes the Pulsar producer
func (p *Producer) Close() error {
	p.producer.Close()
	p.client.Close()
	return nil
}

