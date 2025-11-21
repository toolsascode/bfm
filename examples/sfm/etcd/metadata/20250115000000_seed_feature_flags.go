package metadata

import (
	"bfm/api/migrations"
	_ "embed"
)

//go:embed 20250115000000_seed_feature_flags.up.json
var upSQL string

//go:embed 20250115000000_seed_feature_flags.down.json
var downSQL string

func init() {
	migration := &migrations.MigrationScript{
		Schema:     "", // Dynamic - provided in request
		Version:    "20250115000000",
		Name:       "seed_feature_flags",
		Connection: "metadata",
		Backend:    "etcd",
		UpSQL:      upSQL,
		DownSQL:    downSQL,
	}
	migrations.GlobalRegistry.Register(migration)
}
