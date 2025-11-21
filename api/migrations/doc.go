// Package migrations provides the public API for registering and managing database migrations.
// It exports types, constants, and registry accessors that migration files use to register
// themselves with the global migration registry.
//
// The migrations package is designed to allow migration files outside the bfm module
// to register migrations by importing this package and using the exported types and registry.
//
// Example usage in a migration file:
//
//	package main
//
//	import (
//		"bfm/api/migrations"
//		_ "embed"
//	)
//
//	//go:embed migration.up.sql
//	var upSQL string
//
//	//go:embed migration.down.sql
//	var downSQL string
//
//	func init() {
//		migration := &migrations.MigrationScript{
//			Version:    "20250101120000",
//			Name:       "create_users",
//			Connection: "core",
//			Backend:    "postgresql",
//			UpSQL:      upSQL,
//			DownSQL:    downSQL,
//		}
//		migrations.GlobalRegistry.Register(migration)
//	}
package migrations
