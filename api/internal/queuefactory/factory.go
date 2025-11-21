package queuefactory

import (
	"bfm/api/internal/queue"
	"bfm/api/internal/queue/kafka"
	"bfm/api/internal/queue/pulsar"
	"fmt"
	"strings"
)

// QueueConfig holds configuration for creating a queue
type QueueConfig struct {
	Type               string   // "kafka" or "pulsar"
	KafkaBrokers       []string // Kafka broker addresses
	KafkaTopic         string   // Kafka topic name
	KafkaGroupID       string   // Kafka consumer group ID
	PulsarURL          string   // Pulsar service URL
	PulsarTopic        string   // Pulsar topic name
	PulsarSubscription string   // Pulsar subscription name
}

// NewQueue creates a new queue based on the configuration
func NewQueue(config *QueueConfig) (queue.Queue, error) {
	queueType := strings.ToLower(config.Type)
	if queueType == "" {
		queueType = "kafka" // Default to Kafka
	}

	switch queueType {
	case "kafka":
		if len(config.KafkaBrokers) == 0 {
			return nil, fmt.Errorf("Kafka brokers are required")
		}
		if config.KafkaTopic == "" {
			return nil, fmt.Errorf("Kafka topic is required")
		}
		if config.KafkaGroupID == "" {
			config.KafkaGroupID = "bfm-migration-workers"
		}
		return kafka.NewQueue(config.KafkaBrokers, config.KafkaTopic, config.KafkaGroupID), nil

	case "pulsar":
		if config.PulsarURL == "" {
			return nil, fmt.Errorf("Pulsar URL is required")
		}
		if config.PulsarTopic == "" {
			return nil, fmt.Errorf("Pulsar topic is required")
		}
		if config.PulsarSubscription == "" {
			config.PulsarSubscription = "bfm-migration-workers"
		}
		return pulsar.NewQueue(config.PulsarURL, config.PulsarTopic, config.PulsarSubscription)

	default:
		return nil, fmt.Errorf("unsupported queue type: %s (supported: kafka, pulsar)", config.Type)
	}
}
