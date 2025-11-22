package config

import (
	"os"
	"testing"
)

func TestGetEnvOrDefault(t *testing.T) {
	key := "TEST_ENV_VAR"
	originalValue := os.Getenv(key)
	defer func() {
		if originalValue != "" {
			os.Setenv(key, originalValue)
		} else {
			os.Unsetenv(key)
		}
	}()

	tests := []struct {
		name         string
		envValue     string
		defaultValue string
		want         string
	}{
		{
			name:         "env var set",
			envValue:     "env-value",
			defaultValue: "default-value",
			want:         "env-value",
		},
		{
			name:         "env var not set",
			envValue:     "",
			defaultValue: "default-value",
			want:         "default-value",
		},
		{
			name:         "empty default",
			envValue:     "",
			defaultValue: "",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(key, tt.envValue)
			} else {
				os.Unsetenv(key)
			}

			got := getEnvOrDefault(key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"BFM_API_TOKEN",
		"BFM_HTTP_PORT",
		"BFM_GRPC_PORT",
		"BFM_STATE_BACKEND",
		"BFM_STATE_DB_HOST",
		"BFM_STATE_DB_PORT",
		"BFM_STATE_DB_USERNAME",
		"BFM_STATE_DB_PASSWORD",
		"BFM_STATE_DB_NAME",
		"BFM_STATE_SCHEMA",
		"BFM_QUEUE_ENABLED",
		"BFM_QUEUE_TYPE",
		"BFM_QUEUE_KAFKA_BROKERS",
		"BFM_QUEUE_KAFKA_HOST",
		"BFM_QUEUE_KAFKA_PORT",
		"BFM_QUEUE_KAFKA_TOPIC",
		"BFM_QUEUE_KAFKA_GROUP_ID",
		"BFM_QUEUE_PULSAR_URL",
		"BFM_QUEUE_PULSAR_TOPIC",
		"BFM_QUEUE_PULSAR_SUBSCRIPTION",
	}

	for _, key := range envVars {
		if val := os.Getenv(key); val != "" {
			originalEnv[key] = val
		}
	}

	// Cleanup function
	defer func() {
		for key, val := range originalEnv {
			os.Setenv(key, val)
		}
		for _, key := range envVars {
			if _, exists := originalEnv[key]; !exists {
				os.Unsetenv(key)
			}
		}
		// Clean up connection env vars
		os.Clearenv()
		for key, val := range originalEnv {
			os.Setenv(key, val)
		}
	}()

	tests := []struct {
		name        string
		envSetup    func()
		wantErr     bool
		errContains string
		validate    func(*testing.T, *Config)
	}{
		{
			name: "minimal valid config",
			envSetup: func() {
				os.Setenv("BFM_API_TOKEN", "test-token")
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Server.APIToken != "test-token" {
					t.Errorf("Expected APIToken = test-token, got %v", cfg.Server.APIToken)
				}
				if cfg.Server.HTTPPort != "7070" {
					t.Errorf("Expected default HTTPPort = 7070, got %v", cfg.Server.HTTPPort)
				}
				if cfg.Server.GRPCPort != "9090" {
					t.Errorf("Expected default GRPCPort = 9090, got %v", cfg.Server.GRPCPort)
				}
			},
		},
		{
			name: "missing required BFM_API_TOKEN",
			envSetup: func() {
				os.Unsetenv("BFM_API_TOKEN")
			},
			wantErr:     true,
			errContains: "BFM_API_TOKEN environment variable is required",
		},
		{
			name: "custom server ports",
			envSetup: func() {
				os.Setenv("BFM_API_TOKEN", "test-token")
				os.Setenv("BFM_HTTP_PORT", "8080")
				os.Setenv("BFM_GRPC_PORT", "9091")
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Server.HTTPPort != "8080" {
					t.Errorf("Expected HTTPPort = 8080, got %v", cfg.Server.HTTPPort)
				}
				if cfg.Server.GRPCPort != "9091" {
					t.Errorf("Expected GRPCPort = 9091, got %v", cfg.Server.GRPCPort)
				}
			},
		},
		{
			name: "state database config",
			envSetup: func() {
				os.Setenv("BFM_API_TOKEN", "test-token")
				os.Setenv("BFM_STATE_BACKEND", "postgresql")
				os.Setenv("BFM_STATE_DB_HOST", "localhost")
				os.Setenv("BFM_STATE_DB_PORT", "5432")
				os.Setenv("BFM_STATE_DB_USERNAME", "postgres")
				os.Setenv("BFM_STATE_DB_PASSWORD", "password")
				os.Setenv("BFM_STATE_DB_NAME", "migrations")
				os.Setenv("BFM_STATE_SCHEMA", "public")
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.StateDB.Type != "postgresql" {
					t.Errorf("Expected StateDB.Type = postgresql, got %v", cfg.StateDB.Type)
				}
				if cfg.StateDB.Host != "localhost" {
					t.Errorf("Expected StateDB.Host = localhost, got %v", cfg.StateDB.Host)
				}
				if cfg.StateDB.Port != "5432" {
					t.Errorf("Expected StateDB.Port = 5432, got %v", cfg.StateDB.Port)
				}
				if cfg.StateDB.Username != "postgres" {
					t.Errorf("Expected StateDB.Username = postgres, got %v", cfg.StateDB.Username)
				}
				if cfg.StateDB.Password != "password" {
					t.Errorf("Expected StateDB.Password = password, got %v", cfg.StateDB.Password)
				}
				if cfg.StateDB.Database != "migrations" {
					t.Errorf("Expected StateDB.Database = migrations, got %v", cfg.StateDB.Database)
				}
				if cfg.StateDB.Schema != "public" {
					t.Errorf("Expected StateDB.Schema = public, got %v", cfg.StateDB.Schema)
				}
			},
		},
		{
			name: "queue config - kafka",
			envSetup: func() {
				os.Setenv("BFM_API_TOKEN", "test-token")
				os.Setenv("BFM_QUEUE_ENABLED", "true")
				os.Setenv("BFM_QUEUE_TYPE", "kafka")
				os.Setenv("BFM_QUEUE_KAFKA_BROKERS", "localhost:9092,localhost:9093")
				os.Setenv("BFM_QUEUE_KAFKA_TOPIC", "migrations")
				os.Setenv("BFM_QUEUE_KAFKA_GROUP_ID", "workers")
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if !cfg.Queue.Enabled {
					t.Errorf("Expected Queue.Enabled = true, got %v", cfg.Queue.Enabled)
				}
				if cfg.Queue.Type != "kafka" {
					t.Errorf("Expected Queue.Type = kafka, got %v", cfg.Queue.Type)
				}
				if len(cfg.Queue.KafkaBrokers) != 2 {
					t.Errorf("Expected 2 Kafka brokers, got %v", len(cfg.Queue.KafkaBrokers))
				}
				if cfg.Queue.KafkaTopic != "migrations" {
					t.Errorf("Expected KafkaTopic = migrations, got %v", cfg.Queue.KafkaTopic)
				}
				if cfg.Queue.KafkaGroupID != "workers" {
					t.Errorf("Expected KafkaGroupID = workers, got %v", cfg.Queue.KafkaGroupID)
				}
			},
		},
		{
			name: "queue config - kafka with host/port",
			envSetup: func() {
				os.Setenv("BFM_API_TOKEN", "test-token")
				os.Setenv("BFM_QUEUE_ENABLED", "true")
				os.Setenv("BFM_QUEUE_TYPE", "kafka")
				os.Setenv("BFM_QUEUE_KAFKA_HOST", "kafka-host")
				os.Setenv("BFM_QUEUE_KAFKA_PORT", "9094")
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if len(cfg.Queue.KafkaBrokers) != 1 {
					t.Errorf("Expected 1 Kafka broker, got %v", len(cfg.Queue.KafkaBrokers))
				}
				if cfg.Queue.KafkaBrokers[0] != "kafka-host:9094" {
					t.Errorf("Expected Kafka broker = kafka-host:9094, got %v", cfg.Queue.KafkaBrokers[0])
				}
			},
		},
		{
			name: "queue config - pulsar",
			envSetup: func() {
				os.Setenv("BFM_API_TOKEN", "test-token")
				os.Setenv("BFM_QUEUE_ENABLED", "true")
				os.Setenv("BFM_QUEUE_TYPE", "pulsar")
				os.Setenv("BFM_QUEUE_PULSAR_URL", "pulsar://localhost:6650")
				os.Setenv("BFM_QUEUE_PULSAR_TOPIC", "migrations")
				os.Setenv("BFM_QUEUE_PULSAR_SUBSCRIPTION", "workers")
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Queue.Type != "pulsar" {
					t.Errorf("Expected Queue.Type = pulsar, got %v", cfg.Queue.Type)
				}
				if cfg.Queue.PulsarURL != "pulsar://localhost:6650" {
					t.Errorf("Expected PulsarURL = pulsar://localhost:6650, got %v", cfg.Queue.PulsarURL)
				}
				if cfg.Queue.PulsarTopic != "migrations" {
					t.Errorf("Expected PulsarTopic = migrations, got %v", cfg.Queue.PulsarTopic)
				}
				if cfg.Queue.PulsarSubscription != "workers" {
					t.Errorf("Expected PulsarSubscription = workers, got %v", cfg.Queue.PulsarSubscription)
				}
			},
		},
		{
			name: "connection config",
			envSetup: func() {
				os.Setenv("BFM_API_TOKEN", "test-token")
				os.Setenv("POSTGRES_BACKEND", "postgresql")
				os.Setenv("POSTGRES_DB_HOST", "postgres-host")
				os.Setenv("POSTGRES_DB_PORT", "5432")
				os.Setenv("POSTGRES_DB_USERNAME", "postgres")
				os.Setenv("POSTGRES_DB_PASSWORD", "postgres-password")
				os.Setenv("POSTGRES_DB_NAME", "postgres-db")
				os.Setenv("POSTGRES_SCHEMA", "public")
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				conn, exists := cfg.Connections["postgres"]
				if !exists {
					t.Errorf("Expected connection 'postgres' to exist")
					return
				}
				if conn.Backend != "postgresql" {
					t.Errorf("Expected Backend = postgresql, got %v", conn.Backend)
				}
				if conn.Host != "postgres-host" {
					t.Errorf("Expected Host = postgres-host, got %v", conn.Host)
				}
				if conn.Port != "5432" {
					t.Errorf("Expected Port = 5432, got %v", conn.Port)
				}
				if conn.Username != "postgres" {
					t.Errorf("Expected Username = postgres, got %v", conn.Username)
				}
				if conn.Password != "postgres-password" {
					t.Errorf("Expected Password = postgres-password, got %v", conn.Password)
				}
				if conn.Database != "postgres-db" {
					t.Errorf("Expected Database = postgres-db, got %v", conn.Database)
				}
				if conn.Schema != "public" {
					t.Errorf("Expected Schema = public, got %v", conn.Schema)
				}
			},
		},
		{
			name: "connection config with extra fields",
			envSetup: func() {
				os.Setenv("BFM_API_TOKEN", "test-token")
				os.Setenv("MYSQL_BACKEND", "mysql")
				os.Setenv("MYSQL_SSL_MODE", "require")
				os.Setenv("MYSQL_TIMEOUT", "30")
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *Config) {
				conn, exists := cfg.Connections["mysql"]
				if !exists {
					t.Errorf("Expected connection 'mysql' to exist")
					return
				}
				if conn.Extra["SSL_MODE"] != "require" {
					t.Errorf("Expected Extra[SSL_MODE] = require, got %v", conn.Extra["SSL_MODE"])
				}
				if conn.Extra["TIMEOUT"] != "30" {
					t.Errorf("Expected Extra[TIMEOUT] = 30, got %v", conn.Extra["TIMEOUT"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean environment
			for _, key := range envVars {
				os.Unsetenv(key)
			}
			// Clean connection env vars
			allEnv := os.Environ()
			for _, env := range allEnv {
				key := env
				for i := 0; i < len(env); i++ {
					if env[i] == '=' {
						key = env[:i]
						break
					}
				}
				if len(key) >= 7 && (key[len(key)-7:] == "_BACKEND" || 
					(len(key) >= 8 && key[len(key)-8:] == "_DB_HOST") ||
					(len(key) >= 8 && key[len(key)-8:] == "_DB_PORT") ||
					(len(key) >= 12 && key[len(key)-12:] == "_DB_USERNAME") ||
					(len(key) >= 12 && key[len(key)-12:] == "_DB_PASSWORD") ||
					(len(key) >= 8 && key[len(key)-8:] == "_DB_NAME") ||
					(len(key) >= 7 && key[len(key)-7:] == "_SCHEMA")) {
					os.Unsetenv(key)
				}
			}

			// Setup test environment
			tt.envSetup()

			// Load config
			cfg, err := LoadFromEnv()

			// Validate error
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadFromEnv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" {
				if err.Error() != tt.errContains {
					t.Errorf("LoadFromEnv() error = %v, want error containing %v", err, tt.errContains)
				}
				return
			}

			// Validate config if no error expected
			if !tt.wantErr && cfg != nil && tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

func TestConfig_Connections(t *testing.T) {
	// Save original environment
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	os.Setenv("BFM_API_TOKEN", "test-token")

	// Test multiple connections
	os.Setenv("POSTGRES_BACKEND", "postgresql")
	os.Setenv("POSTGRES_DB_HOST", "postgres-host")
	os.Setenv("MYSQL_BACKEND", "mysql")
	os.Setenv("MYSQL_DB_HOST", "mysql-host")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if len(cfg.Connections) < 2 {
		t.Errorf("Expected at least 2 connections, got %v", len(cfg.Connections))
	}

	// Cleanup
	os.Unsetenv("POSTGRES_BACKEND")
	os.Unsetenv("POSTGRES_DB_HOST")
	os.Unsetenv("MYSQL_BACKEND")
	os.Unsetenv("MYSQL_DB_HOST")
}

func TestConfig_DefaultValues(t *testing.T) {
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	os.Setenv("BFM_API_TOKEN", "test-token")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	// Check defaults
	if cfg.Server.HTTPPort != "7070" {
		t.Errorf("Expected default HTTPPort = 7070, got %v", cfg.Server.HTTPPort)
	}
	if cfg.Server.GRPCPort != "9090" {
		t.Errorf("Expected default GRPCPort = 9090, got %v", cfg.Server.GRPCPort)
	}
	if cfg.StateDB.Type != "postgresql" {
		t.Errorf("Expected default StateDB.Type = postgresql, got %v", cfg.StateDB.Type)
	}
	if cfg.StateDB.Host != "localhost" {
		t.Errorf("Expected default StateDB.Host = localhost, got %v", cfg.StateDB.Host)
	}
	if cfg.StateDB.Port != "5432" {
		t.Errorf("Expected default StateDB.Port = 5432, got %v", cfg.StateDB.Port)
	}
	if cfg.StateDB.Username != "postgres" {
		t.Errorf("Expected default StateDB.Username = postgres, got %v", cfg.StateDB.Username)
	}
	if cfg.StateDB.Database != "migration_state" {
		t.Errorf("Expected default StateDB.Database = migration_state, got %v", cfg.StateDB.Database)
	}
	if cfg.StateDB.Schema != "public" {
		t.Errorf("Expected default StateDB.Schema = public, got %v", cfg.StateDB.Schema)
	}
	if cfg.Queue.Enabled {
		t.Errorf("Expected default Queue.Enabled = false, got %v", cfg.Queue.Enabled)
	}
	if cfg.Queue.Type != "kafka" {
		t.Errorf("Expected default Queue.Type = kafka, got %v", cfg.Queue.Type)
	}
}

func TestConfig_QueueEnabled(t *testing.T) {
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	os.Setenv("BFM_API_TOKEN", "test-token")

	tests := []struct {
		name     string
		envValue string
		want     bool
	}{
		{"enabled true", "true", true},
		{"enabled false", "false", false},
		{"enabled empty", "", false},
		{"enabled invalid", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("BFM_QUEUE_ENABLED", tt.envValue)
			cfg, err := LoadFromEnv()
			if err != nil {
				t.Fatalf("LoadFromEnv() error = %v", err)
			}
			if cfg.Queue.Enabled != tt.want {
				t.Errorf("Queue.Enabled = %v, want %v", cfg.Queue.Enabled, tt.want)
			}
		})
	}
}

func TestConfig_ConnectionsMap(t *testing.T) {
	originalToken := os.Getenv("BFM_API_TOKEN")
	defer func() {
		if originalToken != "" {
			os.Setenv("BFM_API_TOKEN", originalToken)
		} else {
			os.Unsetenv("BFM_API_TOKEN")
		}
	}()

	os.Setenv("BFM_API_TOKEN", "test-token")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.Connections == nil {
		t.Errorf("Expected Connections map to be initialized")
	}

	// Connections map should be initialized (even if empty)
	if cfg.Connections == nil {
		t.Error("Connections map should be initialized")
	}
}

