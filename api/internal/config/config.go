package config

import (
	"fmt"
	"os"
	"strings"

	"bfm/api/internal/backends"
)

// Config holds the application configuration
type Config struct {
	Server struct {
		HTTPPort string
		GRPCPort string
		APIToken string
	}
	StateDB struct {
		Type     string // "postgresql" or "mysql"
		Host     string
		Port     string
		Username string
		Password string
		Database string
		Schema   string // Configurable schema name
	}
	Queue struct {
		Type               string   // "kafka" or "pulsar"
		KafkaBrokers       []string // Kafka broker addresses
		KafkaTopic         string   // Kafka topic name
		KafkaGroupID       string   // Kafka consumer group ID
		PulsarURL          string   // Pulsar service URL
		PulsarTopic        string   // Pulsar topic name
		PulsarSubscription string   // Pulsar subscription name
		Enabled            bool     // Whether to use queue (false = synchronous execution)
	}
	Connections map[string]*backends.ConnectionConfig
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() (*Config, error) {
	config := &Config{
		Connections: make(map[string]*backends.ConnectionConfig),
	}

	// Server configuration
	config.Server.HTTPPort = getEnvOrDefault("BFM_HTTP_PORT", "7070")
	config.Server.GRPCPort = getEnvOrDefault("BFM_GRPC_PORT", "9090")
	config.Server.APIToken = os.Getenv("BFM_API_TOKEN")
	if config.Server.APIToken == "" {
		return nil, fmt.Errorf("BFM_API_TOKEN environment variable is required")
	}

	// State database configuration
	config.StateDB.Type = getEnvOrDefault("BFM_STATE_BACKEND", "postgresql")
	config.StateDB.Host = getEnvOrDefault("BFM_STATE_DB_HOST", "localhost")
	config.StateDB.Port = getEnvOrDefault("BFM_STATE_DB_PORT", "5432")
	config.StateDB.Username = getEnvOrDefault("BFM_STATE_DB_USERNAME", "postgres")
	config.StateDB.Password = os.Getenv("BFM_STATE_DB_PASSWORD")
	config.StateDB.Database = getEnvOrDefault("BFM_STATE_DB_NAME", "migration_state")
	config.StateDB.Schema = getEnvOrDefault("BFM_STATE_SCHEMA", "public")

	// Queue configuration
	config.Queue.Enabled = getEnvOrDefault("BFM_QUEUE_ENABLED", "false") == "true"
	config.Queue.Type = getEnvOrDefault("BFM_QUEUE_TYPE", "kafka")

	// Kafka configuration
	if kafkaBrokers := os.Getenv("BFM_QUEUE_KAFKA_BROKERS"); kafkaBrokers != "" {
		config.Queue.KafkaBrokers = strings.Split(kafkaBrokers, ",")
	} else {
		kafkaHost := getEnvOrDefault("BFM_QUEUE_KAFKA_HOST", "localhost")
		kafkaPort := getEnvOrDefault("BFM_QUEUE_KAFKA_PORT", "9092")
		config.Queue.KafkaBrokers = []string{fmt.Sprintf("%s:%s", kafkaHost, kafkaPort)}
	}
	config.Queue.KafkaTopic = getEnvOrDefault("BFM_QUEUE_KAFKA_TOPIC", "bfm-migrations")
	config.Queue.KafkaGroupID = getEnvOrDefault("BFM_QUEUE_KAFKA_GROUP_ID", "bfm-migration-workers")

	// Pulsar configuration
	config.Queue.PulsarURL = getEnvOrDefault("BFM_QUEUE_PULSAR_URL", "pulsar://localhost:6650")
	config.Queue.PulsarTopic = getEnvOrDefault("BFM_QUEUE_PULSAR_TOPIC", "bfm-migrations")
	config.Queue.PulsarSubscription = getEnvOrDefault("BFM_QUEUE_PULSAR_SUBSCRIPTION", "bfm-migration-workers")

	// Load connection configurations
	// Look for patterns like {CONNECTION}_BACKEND, {CONNECTION}_DB_HOST, etc.
	envVars := os.Environ()
	connectionNames := make(map[string]bool)

	for _, envVar := range envVars {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		// Check for {CONNECTION}_BACKEND pattern
		if strings.HasSuffix(key, "_BACKEND") {
			connectionName := strings.TrimSuffix(key, "_BACKEND")
			connectionName = strings.ToLower(connectionName)
			connectionNames[connectionName] = true

			if config.Connections[connectionName] == nil {
				config.Connections[connectionName] = &backends.ConnectionConfig{
					Backend: value,
					Extra:   make(map[string]string),
				}
			} else {
				config.Connections[connectionName].Backend = value
			}
		}
	}

	// Load connection-specific configs
	for connectionName := range connectionNames {
		prefix := strings.ToUpper(connectionName) + "_"
		conn := config.Connections[connectionName]

		conn.Host = getEnvOrDefault(prefix+"DB_HOST", "")
		conn.Port = getEnvOrDefault(prefix+"DB_PORT", "")
		conn.Username = getEnvOrDefault(prefix+"DB_USERNAME", "")
		conn.Password = os.Getenv(prefix + "DB_PASSWORD")
		conn.Database = getEnvOrDefault(prefix+"DB_NAME", "")
		conn.Schema = getEnvOrDefault(prefix+"SCHEMA", "")

		// Load any extra configs
		for _, envVar := range envVars {
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := parts[0]
			value := parts[1]

			if strings.HasPrefix(key, prefix) && !strings.HasPrefix(key, prefix+"DB_") && key != prefix+"BACKEND" && key != prefix+"SCHEMA" {
				extraKey := strings.TrimPrefix(key, prefix)
				conn.Extra[extraKey] = value
			}
		}
	}

	return config, nil
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
