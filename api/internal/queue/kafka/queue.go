package kafka

import (
	"context"
	"fmt"
	"github.com/toolsascode/bfm/api/internal/queue"
)

// Queue implements queue.Queue using Kafka
type Queue struct {
	producer *Producer
	consumer *Consumer
}

// NewQueue creates a new Kafka queue with both producer and consumer
func NewQueue(brokers []string, topic, groupID string) *Queue {
	return &Queue{
		producer: NewProducer(brokers, topic),
		consumer: NewConsumer(brokers, topic, groupID),
	}
}

// PublishJob publishes a migration job to Kafka
func (q *Queue) PublishJob(ctx context.Context, job *queue.Job) error {
	return q.producer.PublishJob(ctx, job)
}

// Consume starts consuming jobs from Kafka
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
