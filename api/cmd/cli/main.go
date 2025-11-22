package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	migrationpkg "bfm/api/migrations"

	"github.com/spf13/cobra"
)

type migrationFile struct {
	UpFile      string
	DownFile    string
	Version     string
	Name        string
	Backend     string
	Connection  string
	PackageName string
}

var (
	sfmPath   string
	verbose   bool
	dryRun    bool
	outputDir string
)

var rootCmd = &cobra.Command{
	Use:   "bfm",
	Short: "BfM - Backend for Migrations CLI",
	Long: `BfM (Backend for Migrations) is a CLI tool for managing database migrations.

Generate migration .go files from SQL/JSON migration scripts.
Supports PostgreSQL, GreptimeDB, and etcd backends.`,
	Version: "1.0.0",
}

var buildCmd = &cobra.Command{
	Use:   "build [sfm-path]",
	Short: "Build migration .go files from SQL/JSON scripts",
	Long: `Build generates .go files from migration scripts in the SFM directory.

The SFM directory should follow this structure:
  {sfm_path}/{backend}/{connection}/{version}_{name}.up.sql
  {sfm_path}/{backend}/{connection}/{version}_{name}.down.sql
  {sfm_path}/{backend}/{connection}/{version}_{name}.up.json
  {sfm_path}/{backend}/{connection}/{version}_{name}.down.json

Example:
  bfm build examples/sfm
  bfm build /path/to/sfm --verbose
  bfm build examples/sfm --dry-run`,
	Args: cobra.MaximumNArgs(1),
	RunE: runBuild,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("BfM CLI version %s\n", rootCmd.Version)
	},
}

func init() {
	// Build command flags
	buildCmd.Flags().StringVarP(&sfmPath, "path", "p", "", "Path to SFM directory (default: first argument or ./examples/sfm)")
	buildCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	buildCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be generated without creating files")
	buildCmd.Flags().StringVarP(&outputDir, "output", "o", "", "Output directory (default: same as source files)")

	// Add commands
	rootCmd.AddCommand(buildCmd, versionCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runBuild(cmd *cobra.Command, args []string) error {
	// Determine SFM path
	if len(args) > 0 {
		sfmPath = args[0]
	} else if sfmPath == "" {
		// Default to examples/sfm relative to current directory
		sfmPath = "./examples/sfm"
	}

	// Validate path exists
	if _, err := os.Stat(sfmPath); os.IsNotExist(err) {
		return fmt.Errorf("SFM path does not exist: %s", sfmPath)
	}

	if verbose {
		fmt.Printf("Scanning SFM directory: %s\n", sfmPath)
	}

	if dryRun {
		fmt.Println("DRY RUN MODE - No files will be created")
	}

	// Build migrations
	if err := buildMigrations(sfmPath); err != nil {
		return err
	}

	if verbose {
		fmt.Println("\nBuild completed successfully!")
	}

	return nil
}

func buildMigrations(sfmPath string) error {
	// Walk through SFM directory structure: {sfm_path}/{backend}/{connection}/
	migrations := make(map[string]*migrationFile)
	var migrationCount int

	err := filepath.Walk(sfmPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Look for .up.sql, .down.sql, .up.json, .down.json files
		filename := info.Name()
		var isUp, isDown bool
		var ext string

		if strings.HasSuffix(filename, ".up.sql") {
			isUp = true
			ext = ".sql"
		} else if strings.HasSuffix(filename, ".down.sql") {
			isDown = true
			ext = ".sql"
		} else if strings.HasSuffix(filename, ".up.json") {
			isUp = true
			ext = ".json"
		} else if strings.HasSuffix(filename, ".down.json") {
			isDown = true
			ext = ".json"
		} else {
			return nil
		}

		if verbose {
			fmt.Printf("Found migration file: %s\n", path)
		}

		// Parse filename: {version}_{name}.up.{ext} or {version}_{name}.down.{ext}
		baseName := strings.TrimSuffix(filename, ".up"+ext)
		baseName = strings.TrimSuffix(baseName, ".down"+ext)

		// Extract version (14 digits: YYYYMMDDHHMMSS)
		versionRegex := regexp.MustCompile(`^(\d{14})_(.+)$`)
		matches := versionRegex.FindStringSubmatch(baseName)
		if len(matches) != 3 {
			return fmt.Errorf("invalid filename format: %s (expected: {version}_{name}.up.{ext})", filename)
		}

		version := matches[1]
		name := matches[2]

		// Extract backend and connection from directory path
		// Path structure: {sfm_path}/{backend}/{connection}/{filename}
		relPath, err := filepath.Rel(sfmPath, path)
		if err != nil {
			return err
		}

		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) < 3 {
			return fmt.Errorf("invalid directory structure for %s (expected: {backend}/{connection}/{filename})", path)
		}

		backend := parts[0]
		connection := parts[1]

		// Create key for this migration
		key := fmt.Sprintf("%s/%s/%s_%s", backend, connection, version, name)

		// Get or create migration file entry
		migration, exists := migrations[key]
		if !exists {
			migration = &migrationFile{
				Version:     version,
				Name:        name,
				Backend:     backend,
				Connection:  connection,
				PackageName: sanitizePackageName(connection),
			}
			migrations[key] = migration
			migrationCount++
		}

		// Set up or down file
		if isUp {
			migration.UpFile = filename
		} else if isDown {
			migration.DownFile = filename
		}

		return nil
	})

	if err != nil {
		return err
	}

	if migrationCount == 0 {
		fmt.Println("No migration files found in the specified directory")
		return nil
	}

	if verbose {
		fmt.Printf("\nFound %d migration(s) to process\n", migrationCount)
	}

	// Generate .go files
	tmpl, err := template.New("migration").Parse(migrationpkg.GoFileTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var generatedCount int
	for key, migration := range migrations {
		if migration.UpFile == "" {
			return fmt.Errorf("missing up file for migration: %s", key)
		}

		// Determine down file name if not explicitly found
		if migration.DownFile == "" {
			// Try to find corresponding down file
			upExt := filepath.Ext(migration.UpFile)
			baseName := strings.TrimSuffix(migration.UpFile, ".up"+upExt)
			migration.DownFile = baseName + ".down" + upExt
		}

		// Get directory path for this migration
		var dirPath string
		if outputDir != "" {
			dirPath = filepath.Join(outputDir, migration.Backend, migration.Connection)
		} else {
			dirPath = filepath.Join(sfmPath, migration.Backend, migration.Connection)
		}

		// Generate .go filename
		goFileName := fmt.Sprintf("%s_%s.go", migration.Version, migration.Name)
		goFilePath := filepath.Join(dirPath, goFileName)

		if dryRun {
			fmt.Printf("[DRY RUN] Would generate: %s\n", goFilePath)
			generatedCount++
			continue
		}

		// Create output directory if it doesn't exist
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dirPath, err)
		}

		// Create file
		file, err := os.Create(goFilePath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", goFilePath, err)
		}

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
			PackageName:  migration.PackageName,
			UpFileName:   migration.UpFile,
			DownFileName: migration.DownFile,
			Version:      migration.Version,
			Name:         migration.Name,
			Connection:   migration.Connection,
			Backend:      migration.Backend,
		})

		_ = file.Close()

		if err != nil {
			return fmt.Errorf("failed to generate file %s: %w", goFilePath, err)
		}

		if verbose {
			fmt.Printf("Generated: %s\n", goFilePath)
		} else {
			fmt.Printf("Generated: %s\n", goFilePath)
		}
		generatedCount++
	}

	if !dryRun {
		fmt.Printf("\nSuccessfully generated %d migration file(s)\n", generatedCount)
	} else {
		fmt.Printf("\nWould generate %d migration file(s)\n", generatedCount)
	}

	return nil
}

// sanitizePackageName converts a connection name to a valid Go package name
func sanitizePackageName(name string) string {
	// Replace invalid characters with underscores
	re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	result := re.ReplaceAllString(name, "_")

	// Ensure it doesn't start with a number
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "_" + result
	}

	// Ensure it's not empty
	if result == "" {
		result = "migration"
	}

	return result
}
