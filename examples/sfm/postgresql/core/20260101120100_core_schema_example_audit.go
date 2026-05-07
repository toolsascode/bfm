//go:build ignore

package core

import (
	_ "embed"

	"github.com/toolsascode/bfm/api/migrations"
)

//go:embed 20260101120100_core_schema_example_audit.up.sql
var upSQLCoreSchemaExampleAudit string

//go:embed 20260101120100_core_schema_example_audit.down.sql
var downSQLCoreSchemaExampleAudit string

func init() {
	migration := &migrations.MigrationScript{
		Schema:       "core",
		Version:      "20260101120100",
		Name:         "core_schema_example_audit",
		Connection:   "core",
		Backend:      "postgresql",
		UpSQL:        upSQLCoreSchemaExampleAudit,
		DownSQL:      downSQLCoreSchemaExampleAudit,
		Dependencies: []string{"core_schema_example_settings"},
		StructuredDependencies: []migrations.Dependency{},
	}
	migrations.GlobalRegistry.Register(migration)
}
