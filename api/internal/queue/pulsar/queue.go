package pulsar

import (
	"context"
	"fmt"

	"github.com/toolsascode/bfm/api/internal/queue"
)

// Queue implements queue.Queue using Pulsar
type Queue struct {
	producer *Producer
	consumer *Consumer
}

// NewQueue creates a new Pulsar queue with both producer and consumer
func NewQueue(url, topic, subscriptionName string) (*Queue, error) {
	producer, err := NewProducer(url, topic)
	if err != nil {
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	consumer, err := NewConsumer(url, topic, subscriptionName)
	if err != nil {
		_ = producer.Close()
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	return &Queue{
		producer: producer,
		consumer: consumer,
	}, nil
}

// PublishJob publishes a migration job to Pulsar
func (q *Queue) PublishJob(ctx context.Context, job *queue.Job) error {
	return q.producer.PublishJob(ctx, job)
}

// Consume starts consuming jobs from Pulsar
func (q *Queue) Consume(ctx context.Context, handler queue.JobHandler) error {
	return q.consumer.Consume(ctx, handler)
}

// Close closes both producer and consumer
func (q *Queue) Close() error {
	var errs []error

	if err := q.producer.Close(); err != nil {
		errs = append(errs, err)
	}

	if err := q.consumer.Close(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing queue: %v", errs)
	}

	return nil
}
