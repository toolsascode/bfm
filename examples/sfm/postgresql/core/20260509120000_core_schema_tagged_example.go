//go:build ignore

package core

import (
	_ "embed"

	"github.com/toolsascode/bfm/api/migrations"
)

//go:embed 20260509120000_core_schema_tagged_example.up.sql
var upSQLCoreSchemaTaggedExample string

//go:embed 20260509120000_core_schema_tagged_example.down.sql
var downSQLCoreSchemaTaggedExample string

func init() {
	migration := &migrations.MigrationScript{
		Schema:                 "core",
		Version:                "20260509120000",
		Name:                   "core_schema_tagged_example",
		Connection:             "core",
		Backend:                "postgresql",
		UpSQL:                  upSQLCoreSchemaTaggedExample,
		DownSQL:                downSQLCoreSchemaTaggedExample,
		Dependencies:           []string{},
		StructuredDependencies: []migrations.Dependency{},
		Tags:                   []string{"example=demo", "tier=optional"},
	}
	migrations.GlobalRegistry.Register(migration)
}
