package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"bfm/api/internal/backends"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Backend implements the Backend interface for Etcd
type Backend struct {
	client *clientv3.Client
	config *backends.ConnectionConfig
	prefix string
}

// NewBackend creates a new Etcd backend
func NewBackend() *Backend {
	return &Backend{}
}

// Name returns the backend name
func (b *Backend) Name() string {
	return "etcd"
}

// Connect establishes a connection to Etcd
func (b *Backend) Connect(config *backends.ConnectionConfig) error {
	b.config = config

	// Parse endpoints
	endpoints := []string{fmt.Sprintf("%s:%s", config.Host, config.Port)}
	if config.Extra["endpoints"] != "" {
		endpoints = strings.Split(config.Extra["endpoints"], ",")
		for i, ep := range endpoints {
			endpoints[i] = strings.TrimSpace(ep)
		}
	}

	// Get timeout
	timeout := 5 * time.Second
	if timeoutStr := config.Extra["timeout"]; timeoutStr != "" {
		if parsed, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsed
		}
	}

	// Get prefix
	b.prefix = config.Extra["prefix"]
	if b.prefix == "" {
		b.prefix = "/"
	}
	if !strings.HasSuffix(b.prefix, "/") {
		b.prefix += "/"
	}

	// Create etcd client
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		Username:    config.Username,
		Password:    config.Password,
		DialTimeout: timeout,
	})
	if err != nil {
		return fmt.Errorf("failed to create etcd client: %w", err)
	}

	b.client = client

	// Test connection
	if err := b.HealthCheck(context.Background()); err != nil {
		return fmt.Errorf("failed to connect to etcd: %w", err)
	}

	return nil
}

// Close closes the Etcd connection
func (b *Backend) Close() error {
	if b.client != nil {
		return b.client.Close()
	}
	return nil
}

// CreateSchema creates a namespace/prefix in etcd (no-op, handled by prefix)
func (b *Backend) CreateSchema(ctx context.Context, schemaName string) error {
	// In etcd, schemas are represented as key prefixes
	// We just ensure the prefix structure exists by creating a marker key
	key := b.getSchemaKey(schemaName, ".schema_marker")
	value := fmt.Sprintf(`{"created_at": "%s"}`, time.Now().Format(time.RFC3339))

	_, err := b.client.Put(ctx, key, value)
	if err != nil {
		return fmt.Errorf("failed to create schema marker: %w", err)
	}

	return nil
}

// SchemaExists checks if a schema namespace exists
func (b *Backend) SchemaExists(ctx context.Context, schemaName string) (bool, error) {
	key := b.getSchemaKey(schemaName, "")
	resp, err := b.client.Get(ctx, key, clientv3.WithPrefix(), clientv3.WithLimit(1))
	if err != nil {
		return false, fmt.Errorf("failed to check schema existence: %w", err)
	}

	return len(resp.Kvs) > 0, nil
}

// ExecuteMigration executes a migration script
func (b *Backend) ExecuteMigration(ctx context.Context, migration *backends.MigrationScript) error {
	// For etcd, migrations are key-value operations
	// The UpSQL contains JSON with key-value pairs or operations

	// Parse the migration SQL as JSON operations
	var operations []map[string]interface{}
	if err := json.Unmarshal([]byte(migration.UpSQL), &operations); err != nil {
		// If not JSON, treat as a single key-value operation
		// Format: key=value or JSON object
		if strings.Contains(migration.UpSQL, "=") {
			parts := strings.SplitN(migration.UpSQL, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			fullKey := b.getTableKey(migration.Schema, migration.Table, key)
			_, err := b.client.Put(ctx, fullKey, value)
			return err
		}
		return fmt.Errorf("invalid etcd migration format: %w", err)
	}

	// Execute each operation
	for _, op := range operations {
		opType, ok := op["operation"].(string)
		if !ok {
			opType = "put" // Default operation
		}

		switch opType {
		case "put":
			key, ok := op["key"].(string)
			if !ok {
				return fmt.Errorf("missing key in operation")
			}
			value, ok := op["value"].(string)
			if !ok {
				// Try to marshal as JSON if value is an object
				if valObj, ok := op["value"].(map[string]interface{}); ok {
					valBytes, err := json.Marshal(valObj)
					if err != nil {
						return fmt.Errorf("failed to marshal value: %w", err)
					}
					value = string(valBytes)
				} else {
					return fmt.Errorf("missing value in operation")
				}
			}
			fullKey := b.getTableKey(migration.Schema, migration.Table, key)
			_, err := b.client.Put(ctx, fullKey, value)
			if err != nil {
				return fmt.Errorf("failed to put key %s: %w", key, err)
			}

		case "delete":
			key, ok := op["key"].(string)
			if !ok {
				return fmt.Errorf("missing key in operation")
			}
			fullKey := b.getTableKey(migration.Schema, migration.Table, key)
			_, err := b.client.Delete(ctx, fullKey)
			if err != nil {
				return fmt.Errorf("failed to delete key %s: %w", key, err)
			}

		default:
			return fmt.Errorf("unsupported operation type: %s", opType)
		}
	}

	return nil
}

// HealthCheck verifies the backend is accessible
func (b *Backend) HealthCheck(ctx context.Context) error {
	if b.client == nil {
		return fmt.Errorf("etcd client not initialized")
	}

	// Try to get a key to verify connection
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := b.client.Get(ctx, "/health_check")
	if err != nil && err != context.DeadlineExceeded {
		// Ignore "key not found" errors, just check if we can communicate
		if !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("failed to communicate with etcd: %w", err)
		}
	}

	return nil
}

// getSchemaKey builds a key for a schema
// For etcd, if schemaName is provided, it should be used as the full prefix (not appended to connection prefix)
func (b *Backend) getSchemaKey(schemaName, suffix string) string {
	// If schemaName is provided and looks like a full path (starts with /), use it as the full prefix
	// This takes precedence over the connection prefix to allow absolute paths
	if schemaName != "" && strings.HasPrefix(schemaName, "/") {
		// Normalize the schema path (ensure it ends with /)
		normalizedSchema := schemaName
		if !strings.HasSuffix(normalizedSchema, "/") {
			normalizedSchema += "/"
		}
		// Use schema as the full prefix, ignoring connection prefix
		return normalizedSchema + suffix
	}
	// Otherwise, use connection prefix + schema name
	if schemaName != "" {
		return b.prefix + schemaName + "/" + suffix
	}
	return b.prefix + suffix
}

// getTableKey builds a key for a table within a schema
// For etcd, if schemaName is provided and looks like a full path, use it as the full prefix
// If tableName is nil or empty, only uses schema name
func (b *Backend) getTableKey(schemaName string, tableName *string, key string) string {
	// If schemaName is provided and looks like a full path (starts with /), use it as the full prefix
	// This takes precedence over the connection prefix to allow absolute paths
	if schemaName != "" && strings.HasPrefix(schemaName, "/") {
		// Normalize the schema path (ensure it ends with /)
		normalizedSchema := schemaName
		if !strings.HasSuffix(normalizedSchema, "/") {
			normalizedSchema += "/"
		}
		// Use schema as the full prefix, ignoring connection prefix
		if tableName != nil && *tableName != "" {
			return normalizedSchema + *tableName + "/" + key
		}
		return normalizedSchema + key
	}
	// Otherwise, use connection prefix + schema name
	if tableName == nil || *tableName == "" {
		if schemaName != "" {
			return b.prefix + schemaName + "/" + key
		}
		return b.prefix + key
	}
	return b.prefix + schemaName + "/" + *tableName + "/" + key
}
