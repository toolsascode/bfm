//go:build ignore
package core

import (
	"github.com/toolsascode/bfm/api/migrations"
	_ "embed"
)

//go:embed 20250115000000_bootstrap_solution.up.sql
var upSQL string

//go:embed 20250115000000_bootstrap_solution.down.sql
var downSQL string

func init() {
	migration := &migrations.MigrationScript{
		Schema:       "", // Dynamic - provided in request
		Version:      "20250115000000",
		Name:         "bootstrap_solution",
		Connection:   "core",
		Backend:      "postgresql",
		UpSQL:        upSQL,
		DownSQL:      downSQL,
		Dependencies: []string{  },
		StructuredDependencies: []migrations.Dependency{},
	}
	migrations.GlobalRegistry.Register(migration)
}
