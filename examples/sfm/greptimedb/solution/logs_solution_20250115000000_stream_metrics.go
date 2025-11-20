package solution

import (
	_ "embed"
	"mops/bfm/migrations"
)

//go:embed logs_solution_20250115000000_stream_metrics.sql
var upSQL string

//go:embed logs_solution_20250115000000_stream_metrics_down.sql
var downSQL string

func init() {
	migration := &migrations.MigrationScript{
		Schema:     "", // Dynamic per environment: cli_{environment_id}
		Table:      "solution_streams",
		Version:    "20250115000000",
		Name:       "stream_metrics",
		Connection: "logs",
		Backend:    "greptimedb",
		UpSQL:      upSQL,
		DownSQL:    downSQL,
	}

	migrations.GlobalRegistry.Register(migration)
}
