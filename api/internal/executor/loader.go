package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"bfm/api/internal/backends"
	"bfm/api/internal/logger"
	"bfm/api/internal/registry"
)

// Loader loads migration scripts from the SFM directory
type Loader struct {
	sfmPath      string
	registry     registry.Registry
	executor     *Executor // Optional executor for registering scanned migrations
	seenFiles    map[string]time.Time // Track files we've seen and their mod times
	mu           sync.RWMutex
	watchContext context.Context
	watchCancel  context.CancelFunc
	watching     bool
}

// NewLoader creates a new migration loader
func NewLoader(sfmPath string) *Loader {
	ctx, cancel := context.WithCancel(context.Background())
	return &Loader{
		sfmPath:      sfmPath,
		seenFiles:    make(map[string]time.Time),
		watchContext: ctx,
		watchCancel:  cancel,
	}
}

// SetExecutor sets the executor for registering scanned migrations
func (l *Loader) SetExecutor(exec *Executor) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.executor = exec
}

// LoadAll loads all migration scripts from the SFM directory structure
// It reads .go files to extract metadata, then reads the corresponding SQL/JSON files
// and registers migrations directly in the registry.
func (l *Loader) LoadAll(reg registry.Registry) error {
	l.registry = reg

	if l.sfmPath == "" {
		// Default to ../sfm relative to bfm
		l.sfmPath = "../sfm"
	}

	// Initial load - force load all existing files
	if err := l.scanAndLoadAll(); err != nil {
		return err
	}

	return nil
}

// scanAndLoadAll scans and loads all migration files (used for initial load)
func (l *Loader) scanAndLoadAll() error {
	if l.sfmPath == "" {
		return nil
	}

	// Check if directory exists
	if _, err := os.Stat(l.sfmPath); os.IsNotExist(err) {
		logger.Warnf("SFM directory does not exist: %s", l.sfmPath)
		return nil
	}

	var loadedCount int
	err := filepath.Walk(l.sfmPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only process .go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Verify directory structure: sfm/{backend}/{connection}/{version}_{name}.go
		relPath, err := filepath.Rel(l.sfmPath, path)
		if err != nil {
			return err
		}

		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) < 3 {
			return nil
		}

		filename := parts[len(parts)-1]
		filenameWithoutExt := strings.TrimSuffix(filename, ".go")

		// Verify filename format: {version}_{name}.go where version is 14 digits
		versionRegex := regexp.MustCompile(`^(\d{14})_(.+)$`)
		matches := versionRegex.FindStringSubmatch(filenameWithoutExt)
		if len(matches) != 3 {
			return nil
		}

		version := matches[1]
		name := matches[2]
		backend := parts[0]
		connection := parts[1]

		// Load the migration
		if l.registry != nil {
			if err := l.loadMigrationFromFile(path, backend, connection, version, name); err != nil {
				logger.Warnf("Failed to load migration from %s: %v", path, err)
				return nil // Continue with other files
			}
			loadedCount++
		}

		// Track this file
		modTime := info.ModTime()
		l.mu.Lock()
		l.seenFiles[path] = modTime
		l.mu.Unlock()

		return nil
	})

	if err != nil {
		return fmt.Errorf("error scanning SFM directory: %w", err)
	}

	logger.Infof("Loaded %d migration(s) from %s", loadedCount, l.sfmPath)
	return nil
}

// StartWatching starts a background goroutine that checks for new migration files every minute
func (l *Loader) StartWatching() {
	if l.watching {
		return // Already watching
	}

	l.mu.Lock()
	l.watching = true
	l.mu.Unlock()

	logger.Info("Starting migration file watcher (checking every minute)")

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-l.watchContext.Done():
				logger.Info("Migration file watcher stopped")
				return
			case <-ticker.C:
				if err := l.scanAndLoad(); err != nil {
					logger.Warnf("Error scanning for new migrations: %v", err)
				}
			}
		}
	}()
}

// StopWatching stops the background file watcher
func (l *Loader) StopWatching() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.watching {
		return
	}

	l.watchCancel()
	l.watching = false
}

// scanAndLoad scans the SFM directory and loads any new migration files
func (l *Loader) scanAndLoad() error {
	if l.sfmPath == "" {
		return nil
	}

	// Check if directory exists
	if _, err := os.Stat(l.sfmPath); os.IsNotExist(err) {
		return nil // Directory doesn't exist, skip
	}

	// Walk through the SFM directory structure
	// Structure: sfm/{backend}/{connection}/{version}_{name}.go
	newFiles := make(map[string]time.Time)

	err := filepath.Walk(l.sfmPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only process .go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip test files
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Verify directory structure: sfm/{backend}/{connection}/{version}_{name}.go
		relPath, err := filepath.Rel(l.sfmPath, path)
		if err != nil {
			return err
		}

		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) < 3 {
			// Not in expected structure, skip
			return nil
		}

		filename := parts[len(parts)-1]
		filenameWithoutExt := strings.TrimSuffix(filename, ".go")

		// Verify filename format: {version}_{name}.go where version is 14 digits
		// Extract version (should be a timestamp like 20250101120000)
		versionRegex := regexp.MustCompile(`^(\d{14})_(.+)$`)
		matches := versionRegex.FindStringSubmatch(filenameWithoutExt)
		if len(matches) != 3 {
			// Skip files that don't match the expected format
			return nil
		}

		// Track this file
		modTime := info.ModTime()
		newFiles[path] = modTime

		// Check if this is a new or modified file
		l.mu.RLock()
		seenTime, seen := l.seenFiles[path]
		l.mu.RUnlock()

		version := matches[1]
		name := matches[2]

		// Extract backend and connection from directory path
		backend := parts[0]
		connection := parts[1]

		// Check if this is a new or modified file that needs to be loaded
		needsLoad := false
		if !seen {
			needsLoad = true
			logger.Infof("New migration file detected: %s (version: %s, name: %s)", path, version, name)
		} else if modTime.After(seenTime) {
			needsLoad = true
			logger.Infof("Migration file modified: %s (version: %s, name: %s)", path, version, name)
		}

		// Load the migration if needed
		if needsLoad && l.registry != nil {
			if err := l.loadMigrationFromFile(path, backend, connection, version, name); err != nil {
				logger.Warnf("Failed to load migration from %s: %v", path, err)
			} else {
				// Migration loaded successfully, it will be registered in database by loadMigrationFromFile
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error scanning SFM directory: %w", err)
	}

	// Update seen files map
	l.mu.Lock()
	l.seenFiles = newFiles
	l.mu.Unlock()

	return nil
}

// loadMigrationFromFile loads a migration by reading the .go file and corresponding SQL/JSON files
func (l *Loader) loadMigrationFromFile(goFilePath, backend, connection, version, name string) error {
	// Determine file extensions based on backend
	var upExt, downExt string
	if backend == "etcd" {
		upExt = ".up.json"
		downExt = ".down.json"
	} else {
		upExt = ".up.sql"
		downExt = ".down.sql"
	}

	// Build file paths
	dir := filepath.Dir(goFilePath)
	baseName := fmt.Sprintf("%s_%s", version, name)
	upFile := filepath.Join(dir, baseName+upExt)
	downFile := filepath.Join(dir, baseName+downExt)

	// Read up migration file
	upSQL, err := os.ReadFile(upFile)
	if err != nil {
		return fmt.Errorf("failed to read up migration file %s: %w", upFile, err)
	}

	// Read down migration file (optional - may not exist)
	downSQL, err := os.ReadFile(downFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read down migration file %s: %w", downFile, err)
	}

	// Create and register migration
	migration := &backends.MigrationScript{
		Schema:     "", // Dynamic - provided in request
		Version:    version,
		Name:       name,
		Connection: connection,
		Backend:    backend,
		UpSQL:      string(upSQL),
		DownSQL:    string(downSQL),
	}

	if err := l.registry.Register(migration); err != nil {
		return fmt.Errorf("failed to register migration: %w", err)
	}

	// Register scanned migration in migrations_list table if executor is available
	if l.executor != nil {
		// Generate migration ID (format: {connection}_{version}_{name} since schema is dynamic)
		migrationID := fmt.Sprintf("%s_%s_%s", connection, version, name)
		
		// Register in database (schema and table are empty for now, will be set on execution)
		ctx := context.Background()
		if err := l.executor.RegisterScannedMigration(ctx, migrationID, "", "", version, name, connection, backend); err != nil {
			// Log warning but don't fail - migration is still registered in memory
			logger.Warnf("Failed to register scanned migration in database: %v", err)
		}
	}

	logger.Infof("Registered migration: %s_%s_%s (backend: %s, connection: %s)", connection, version, name, backend, connection)
	return nil
}

// isNumeric checks if a string contains only digits
func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
