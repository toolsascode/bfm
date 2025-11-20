package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"mops/bfm/internal/logger"
	"mops/bfm/internal/queue"
)

// Producer implements queue.Producer using Kafka
type Producer struct {
	writer *kafka.Writer
	topic  string
}

// NewProducer creates a new Kafka producer
func NewProducer(brokers []string, topic string) *Producer {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		WriteTimeout: 10 * time.Second,
		RequiredAcks: kafka.RequireOne,
	}

	return &Producer{
		writer: writer,
		topic:  topic,
	}
}

// PublishJob publishes a migration job to Kafka
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

	// Create Kafka message
	message := kafka.Message{
		Key:   []byte(job.ID),
		Value: jobData,
		Headers: []kafka.Header{
			{Key: "job-id", Value: []byte(job.ID)},
			{Key: "connection", Value: []byte(job.Connection)},
		},
	}

	// Publish message
	err = p.writer.WriteMessages(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to write message to Kafka: %w", err)
	}

	logger.Infof("Published migration job %s to Kafka topic %s", job.ID, p.topic)
	return nil
}

// Close closes the Kafka producer
func (p *Producer) Close() error {
	return p.writer.Close()
}

