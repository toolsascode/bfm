package solution

import (
	_ "embed"
	"mops/bfm/migrations"
)

//go:embed metadata_solution_20250115000000_seed_feature_flags.json
var upSQL string

//go:embed metadata_solution_20250115000000_seed_feature_flags_down.json
var downSQL string

func init() {
	migration := &migrations.MigrationScript{
		Schema:     "metadata",
		Table:      "solution_feature_flags",
		Version:    "20250115000000",
		Name:       "seed_feature_flags",
		Connection: "metadata",
		Backend:    "etcd",
		UpSQL:      upSQL,
		DownSQL:    downSQL,
	}

	migrations.GlobalRegistry.Register(migration)
}
