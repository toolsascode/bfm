package logs

import (
	_ "embed"

	"github.com/toolsascode/bfm/api/migrations"
)

//go:embed 20250115000000_stream_metrics.up.sql
var upSQL string

//go:embed 20250115000000_stream_metrics.down.sql
var downSQL string

func init() {
	migration := &migrations.MigrationScript{
		Schema:                 "", // Dynamic - provided in request
		Version:                "20250115000000",
		Name:                   "stream_metrics",
		Connection:             "logs",
		Backend:                "greptimedb",
		UpSQL:                  upSQL,
		DownSQL:                downSQL,
		Dependencies:           []string{}, // No dependencies
		StructuredDependencies: []migrations.Dependency{},
	}
	migrations.GlobalRegistry.Register(migration)
}
