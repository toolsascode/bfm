package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mops/bfm/internal/backends"
	"mops/bfm/internal/registry"
)

// Loader loads migration scripts from the SFM directory
type Loader struct {
	sfmPath string
}

// NewLoader creates a new migration loader
func NewLoader(sfmPath string) *Loader {
	return &Loader{
		sfmPath: sfmPath,
	}
}

// LoadAll loads all migration scripts from the SFM directory structure
func (l *Loader) LoadAll(reg registry.Registry) error {
	if l.sfmPath == "" {
		// Default to ../sfm relative to bfm
		l.sfmPath = "../sfm"
	}

	// Walk through the SFM directory structure
	// Structure: sfm/{backend}/{connection}/{schema}_{table}_{version}_{name}.go
	return filepath.Walk(l.sfmPath, func(path string, info os.FileInfo, err error) error {
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

		// Parse the file path to extract metadata
		// Expected: sfm/{backend}/{connection}/{schema}_{table}_{version}_{name}.go
		relPath, err := filepath.Rel(l.sfmPath, path)
		if err != nil {
			return err
		}

		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) < 3 {
			// Not in expected structure, skip
			return nil
		}

		backend := parts[0]
		connection := parts[1]
		filename := parts[len(parts)-1]

		// Parse filename: {schema}_{table}_{version}_{name}.go
		filenameWithoutExt := strings.TrimSuffix(filename, ".go")
		filenameParts := strings.Split(filenameWithoutExt, "_")
		if len(filenameParts) < 4 {
			return fmt.Errorf("invalid migration filename format: %s (expected: {schema}_{table}_{version}_{name}.go)", filename)
		}

		// Extract version (should be a timestamp like 20250101120000)
		// Find the version part (14 digits)
		var version string
		var versionIndex int
		for i, part := range filenameParts {
			if len(part) == 14 && isNumeric(part) {
				version = part
				versionIndex = i
				break
			}
		}

		if version == "" {
			return fmt.Errorf("could not find version in filename: %s", filename)
		}

		// Schema is everything before version
		schema := strings.Join(filenameParts[:versionIndex], "_")
		// Table is the part after schema, before version
		// Actually, let's assume: schema is first part, table is second part
		if versionIndex < 2 {
			return fmt.Errorf("invalid filename structure: %s", filename)
		}
		schema = filenameParts[0]
		table := strings.Join(filenameParts[1:versionIndex], "_")
		// Name is everything after version
		name := strings.Join(filenameParts[versionIndex+1:], "_")

		// Read the SQL/JSON files from disk
		// Find the directory containing the .go file
		goFileDir := filepath.Dir(path)
		filenameBase := strings.TrimSuffix(filename, ".go")

		// Determine file extension based on backend
		var upExt, downExt string
		if backend == "etcd" {
			upExt = ".json"
			downExt = "_down.json"
		} else {
			upExt = ".sql"
			downExt = "_down.sql"
		}

		// Read up migration file
		upFile := filepath.Join(goFileDir, filenameBase+upExt)
		upSQL, err := os.ReadFile(upFile)
		if err != nil {
			return fmt.Errorf("failed to read up migration file %s: %w", upFile, err)
		}

		// Read down migration file (optional - may not exist)
		downFile := filepath.Join(goFileDir, filenameBase+downExt)
		downSQL, err := os.ReadFile(downFile)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to read down migration file %s: %w", downFile, err)
		}

		// Create and register the migration
		migration := &backends.MigrationScript{
			Schema:     schema,
			Table:      table,
			Version:    version,
			Name:       name,
			Connection: connection,
			Backend:    backend,
			UpSQL:      string(upSQL),
			DownSQL:    string(downSQL),
		}

		// Register the migration
		if err := reg.Register(migration); err != nil {
			return fmt.Errorf("failed to register migration %s: %w", filename, err)
		}

		return nil
	})
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
