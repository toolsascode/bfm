package migrations

const GoFileTemplate = `//go:build ignore
package {{.PackageName}}

import (
	"github.com/toolsascode/bfm/api/migrations"
	_ "embed"
)

//go:embed {{.UpFileName}}
var upSQL string

//go:embed {{.DownFileName}}
var downSQL string

func init() {
	migration := &migrations.MigrationScript{
		Schema:       "", // Dynamic - provided in request
		Version:      "{{.Version}}",
		Name:         "{{.Name}}",
		Connection:   "{{.Connection}}",
		Backend:      "{{.Backend}}",
		UpSQL:        upSQL,
		DownSQL:      downSQL,
		Dependencies: []string{ {{.Dependencies}} },
		StructuredDependencies: []migrations.Dependency{},
	}
	migrations.GlobalRegistry.Register(migration)
}
`
