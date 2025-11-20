package solution

import (
	_ "embed"
	"mops/bfm/migrations"
)

//go:embed core_solution_20250115000000_bootstrap_solution.sql
var upSQL string

//go:embed core_solution_20250115000000_bootstrap_solution_down.sql
var downSQL string

func init() {
	migration := &migrations.MigrationScript{
		Schema:     "core",
		Table:      "solution_runs",
		Version:    "20250115000000",
		Name:       "bootstrap_solution",
		Connection: "core",
		Backend:    "postgresql",
		UpSQL:      upSQL,
		DownSQL:    downSQL,
	}

	migrations.GlobalRegistry.Register(migration)
}
