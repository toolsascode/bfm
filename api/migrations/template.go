package migrations

// GoFileTemplate is the template for generating migration .go files
// It is used by both the CLI tool and the loader to create migration files
const GoFileTemplate = `package {{.PackageName}}

import (
	"bfm/api/migrations"
	_ "embed"
)

//go:embed {{.UpFileName}}
var upSQL string

//go:embed {{.DownFileName}}
var downSQL string

func init() {
	migration := &migrations.MigrationScript{
		Schema:     "", // Dynamic - provided in request
		Version:    "{{.Version}}",
		Name:       "{{.Name}}",
		Connection: "{{.Connection}}",
		Backend:    "{{.Backend}}",
		UpSQL:      upSQL,
		DownSQL:    downSQL,
	}
	migrations.GlobalRegistry.Register(migration)
}
`
