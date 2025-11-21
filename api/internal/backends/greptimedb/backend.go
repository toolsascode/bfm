package greptimedb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bfm/api/internal/backends"
)

// Backend implements the Backend interface for GreptimeDB
type Backend struct {
	baseURL  string
	client   *http.Client
	config   *backends.ConnectionConfig
	username string
	password string
}

// NewBackend creates a new GreptimeDB backend
func NewBackend() *Backend {
	return &Backend{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the backend name
func (b *Backend) Name() string {
	return "greptimedb"
}

// Connect establishes a connection to GreptimeDB
func (b *Backend) Connect(config *backends.ConnectionConfig) error {
	b.config = config

	// Build base URL
	protocol := "http"
	if config.Extra["ssl"] == "true" || config.Extra["tls"] == "true" {
		protocol = "https"
	}

	port := config.Port
	if port == "" {
		port = "4001" // Default GreptimeDB HTTP port
	}

	b.baseURL = fmt.Sprintf("%s://%s:%s", protocol, config.Host, port)
	b.username = config.Username
	b.password = config.Password

	// Test connection with health check
	if err := b.HealthCheck(context.Background()); err != nil {
		return fmt.Errorf("failed to connect to GreptimeDB: %w", err)
	}

	return nil
}

// Close closes the GreptimeDB connection (no-op for HTTP client)
func (b *Backend) Close() error {
	// HTTP client doesn't need explicit closing
	return nil
}

// CreateSchema creates a database if it doesn't exist
func (b *Backend) CreateSchema(ctx context.Context, schemaName string) error {
	// In GreptimeDB, "schema" is actually a database
	query := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", quoteIdentifier(schemaName))
	return b.executeSQL(ctx, schemaName, query)
}

// SchemaExists checks if a database exists
func (b *Backend) SchemaExists(ctx context.Context, schemaName string) (bool, error) {
	// Query information_schema to check if database exists
	query := fmt.Sprintf("SHOW DATABASES LIKE '%s'", schemaName)
	result, err := b.querySQL(ctx, "public", query)
	if err != nil {
		return false, fmt.Errorf("failed to check database existence: %w", err)
	}

	// Parse result to see if database exists
	return strings.Contains(result, schemaName), nil
}

// ExecuteMigration executes a migration script
func (b *Backend) ExecuteMigration(ctx context.Context, migration *backends.MigrationScript) error {
	// Determine database name
	dbName := migration.Schema
	if dbName == "" {
		dbName = b.config.Database
	}

	// Ensure database exists
	exists, err := b.SchemaExists(ctx, dbName)
	if err != nil {
		return fmt.Errorf("failed to check database existence: %w", err)
	}
	if !exists {
		if err := b.CreateSchema(ctx, dbName); err != nil {
			return fmt.Errorf("failed to create database: %w", err)
		}
	}

	// Execute migration SQL
	return b.executeSQL(ctx, dbName, migration.UpSQL)
}

// HealthCheck verifies the backend is accessible
func (b *Backend) HealthCheck(ctx context.Context) error {
	// Try a simple query
	_, err := b.querySQL(ctx, "public", "SELECT 1")
	return err
}

// executeSQL executes a SQL statement via HTTP
func (b *Backend) executeSQL(ctx context.Context, dbName, sqlQuery string) error {
	// URL-encode the SQL query
	formData := url.Values{}
	formData.Set("sql", sqlQuery)

	// Build URL with database context
	requestURL := fmt.Sprintf("%s/v1/sql?db=%s", b.baseURL, url.QueryEscape(dbName))

	req, err := http.NewRequestWithContext(ctx, "POST", requestURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if b.username != "" {
		req.SetBasicAuth(b.username, b.password)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to execute SQL: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// querySQL executes a SQL query and returns the result as string
func (b *Backend) querySQL(ctx context.Context, dbName, sqlQuery string) (string, error) {
	formData := url.Values{}
	formData.Set("sql", sqlQuery)

	requestURL := fmt.Sprintf("%s/v1/sql?db=%s", b.baseURL, url.QueryEscape(dbName))

	req, err := http.NewRequestWithContext(ctx, "POST", requestURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if b.username != "" {
		req.SetBasicAuth(b.username, b.password)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to query SQL: status %d, body: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}

// quoteIdentifier quotes a GreptimeDB identifier
func quoteIdentifier(name string) string {
	// GreptimeDB uses backticks for identifiers
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
