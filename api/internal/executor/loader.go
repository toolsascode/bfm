package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	"bfm/api/internal/backends"
	"bfm/api/internal/logger"
	"bfm/api/internal/registry"
	"bfm/api/migrations"
)

// Loader loads migration scripts from the SFM directory
type Loader struct {
	sfmPath      string
	registry     registry.Registry
	executor     *Executor            // Optional executor for registering scanned migrations
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

	// First, scan for SQL/JSON files and auto-create .go files if needed
	// Also loads migrations directly from SQL/JSON if .go file creation fails
	if migrations, err := l.findMigrationFilesFromSQLOrJSON(); err != nil {
		logger.Warnf("Failed to scan for SQL/JSON migration files: %v", err)
	} else {
		createdCount := 0
		loadedCount := 0
		for goFilePath := range migrations {
			if goFilePath != "" {
				createdCount++
			} else {
				loadedCount++
			}
		}
		if createdCount > 0 {
			logger.Infof("Auto-created %d .go file(s) from SQL/JSON files", createdCount)
		}
		if loadedCount > 0 {
			logger.Infof("Loaded %d migration(s) directly from SQL/JSON files (read-only filesystem)", loadedCount)
		}
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

	// First, scan for SQL/JSON files and auto-create .go files if needed
	// Also loads migrations directly from SQL/JSON if .go file creation fails
	if migrations, err := l.findMigrationFilesFromSQLOrJSON(); err != nil {
		logger.Warnf("Failed to scan for SQL/JSON migration files: %v", err)
	} else {
		createdCount := 0
		loadedCount := 0
		for goFilePath := range migrations {
			if goFilePath != "" {
				createdCount++
			} else {
				loadedCount++
			}
		}
		if createdCount > 0 {
			logger.Infof("Auto-created %d .go file(s) from SQL/JSON files", createdCount)
		}
		if loadedCount > 0 {
			logger.Infof("Loaded %d migration(s) directly from SQL/JSON files (read-only filesystem)", loadedCount)
		}
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
	if backend == "etcd" || backend == "mongodb" {
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

// ensureGoFileExists checks if a .go file exists for the given migration files.
// If the .go file doesn't exist but the .up.sql/.up.json and .down.sql/.down.json files do,
// it automatically creates the .go file.
// Returns the goFilePath if it exists or was created, or an empty string if creation failed
// (e.g., read-only filesystem). The error indicates whether SQL/JSON files are missing.
func (l *Loader) ensureGoFileExists(backend, connection, version, name string) (string, error) {
	// Determine file extensions based on backend
	var upExt, downExt string
	if backend == "etcd" || backend == "mongodb" {
		upExt = ".up.json"
		downExt = ".down.json"
	} else {
		upExt = ".up.sql"
		downExt = ".down.sql"
	}

	// Build directory path
	dir := filepath.Join(l.sfmPath, backend, connection)
	baseName := fmt.Sprintf("%s_%s", version, name)
	goFilePath := filepath.Join(dir, baseName+".go")
	upFile := filepath.Join(dir, baseName+upExt)
	downFile := filepath.Join(dir, baseName+downExt)

	// Check if .go file already exists
	if _, err := os.Stat(goFilePath); err == nil {
		return goFilePath, nil // .go file exists, no need to create
	}

	// Check if .up file exists
	if _, err := os.Stat(upFile); os.IsNotExist(err) {
		return "", fmt.Errorf("up migration file does not exist: %s", upFile)
	}

	// Check if .down file exists (required per user requirement)
	if _, err := os.Stat(downFile); os.IsNotExist(err) {
		return "", fmt.Errorf("down migration file does not exist: %s", downFile)
	}

	// Try to create directory if it doesn't exist (may fail on read-only filesystem)
	if err := os.MkdirAll(dir, 0755); err != nil {
		// If directory creation fails, it might be read-only filesystem
		// Return empty string (no .go file) but no error (SQL/JSON files exist)
		logger.Warnf("Cannot create directory %s (filesystem may be read-only): %v", dir, err)
		return "", nil
	}

	// Parse template
	tmpl, err := template.New("goFile").Parse(migrations.GoFileTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// Prepare template data
	upFileName := filepath.Base(upFile)
	downFileName := filepath.Base(downFile)

	// Create .go file
	file, err := os.Create(goFilePath)
	if err != nil {
		// If file creation fails (e.g., read-only filesystem), return empty string
		// but no error since SQL/JSON files exist and can be loaded directly
		logger.Warnf("Cannot create .go file %s (filesystem may be read-only): %v", goFilePath, err)
		return "", nil
	}
	defer file.Close()

	// Execute template
	err = tmpl.Execute(file, struct {
		PackageName  string
		UpFileName   string
		DownFileName string
		Version      string
		Name         string
		Connection   string
		Backend      string
	}{
		PackageName:  connection,
		UpFileName:   upFileName,
		DownFileName: downFileName,
		Version:      version,
		Name:         name,
		Connection:   connection,
		Backend:      backend,
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate file %s: %w", goFilePath, err)
	}

	logger.Infof("Auto-generated .go file: %s", goFilePath)
	return goFilePath, nil
}

// findMigrationFilesFromSQLOrJSON scans for .up.sql or .up.json files and creates corresponding .go files
// Also loads migrations directly from SQL/JSON files if .go file creation fails (e.g., read-only filesystem)
// Returns a map of goFilePath -> (backend, connection, version, name)
// If goFilePath is empty, the migration was loaded directly from SQL/JSON files
func (l *Loader) findMigrationFilesFromSQLOrJSON() (map[string][]string, error) {
	migrations := make(map[string][]string) // goFilePath -> [backend, connection, version, name]

	err := filepath.Walk(l.sfmPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Look for .up.sql or .up.json files
		var isUpFile bool
		var upExt string
		if strings.HasSuffix(path, ".up.sql") {
			isUpFile = true
			upExt = ".up.sql"
		} else if strings.HasSuffix(path, ".up.json") {
			isUpFile = true
			upExt = ".up.json"
		}

		if !isUpFile {
			return nil
		}

		// Verify directory structure: sfm/{backend}/{connection}/{version}_{name}.up.{sql|json}
		relPath, err := filepath.Rel(l.sfmPath, path)
		if err != nil {
			return err
		}

		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) < 3 {
			return nil
		}

		filename := parts[len(parts)-1]
		filenameWithoutExt := strings.TrimSuffix(filename, upExt)

		// Verify filename format: {version}_{name}.up.{sql|json} where version is 14 digits
		versionRegex := regexp.MustCompile(`^(\d{14})_(.+)$`)
		matches := versionRegex.FindStringSubmatch(filenameWithoutExt)
		if len(matches) != 3 {
			return nil
		}

		version := matches[1]
		name := matches[2]
		backend := parts[0]
		connection := parts[1]

		// Check if .go file exists, if not try to create it
		goFilePath, err := l.ensureGoFileExists(backend, connection, version, name)
		if err != nil {
			// Error means SQL/JSON files are missing, skip this migration
			logger.Warnf("Failed to ensure .go file exists for %s: %v", path, err)
			return nil // Continue with other files
		}

		// If goFilePath is empty, .go file creation failed (e.g., read-only filesystem)
		// but SQL/JSON files exist, so load migration directly
		if goFilePath == "" {
			// Load migration directly from SQL/JSON files
			if l.registry != nil {
				// Build the path to the .go file (even though it doesn't exist)
				// loadMigrationFromFile will read SQL/JSON files directly
				dir := filepath.Join(l.sfmPath, backend, connection)
				baseName := fmt.Sprintf("%s_%s", version, name)
				virtualGoPath := filepath.Join(dir, baseName+".go")

				if err := l.loadMigrationFromFile(virtualGoPath, backend, connection, version, name); err != nil {
					logger.Warnf("Failed to load migration directly from SQL/JSON for %s: %v", path, err)
					return nil // Continue with other files
				}
				logger.Infof("Loaded migration directly from SQL/JSON: %s_%s (backend: %s, connection: %s)", version, name, backend, connection)
			}
			// Use empty string as key to indicate migration loaded without .go file
			migrations[""] = []string{backend, connection, version, name}
		} else {
			// Store migration info with goFilePath
			migrations[goFilePath] = []string{backend, connection, version, name}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error scanning for SQL/JSON migration files: %w", err)
	}

	return migrations, nil
}
