//go:build ignore

package core

import (
	_ "embed"

	"github.com/toolsascode/bfm/api/migrations"
)

//go:embed 20260101120000_core_schema_example_settings.up.sql
var upSQLCoreSchemaExampleSettings string

//go:embed 20260101120000_core_schema_example_settings.down.sql
var downSQLCoreSchemaExampleSettings string

func init() {
	migration := &migrations.MigrationScript{
		Schema:                 "core",
		Version:                "20260101120000",
		Name:                   "core_schema_example_settings",
		Connection:             "core",
		Backend:                "postgresql",
		UpSQL:                  upSQLCoreSchemaExampleSettings,
		DownSQL:                downSQLCoreSchemaExampleSettings,
		Dependencies:           []string{},
		StructuredDependencies: []migrations.Dependency{},
	}
	migrations.GlobalRegistry.Register(migration)
}
