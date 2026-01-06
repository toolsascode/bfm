//go:build ignore

package core

import (
	_ "embed"

	"github.com/toolsascode/bfm/api/migrations"
)

//go:embed 20250116000000_create_user_authentication_and_authorization_system_with_role_based_access_control.up.sql
var upSQL string

//go:embed 20250116000000_create_user_authentication_and_authorization_system_with_role_based_access_control.down.sql
var downSQL string

func init() {
	migration := &migrations.MigrationScript{
		Schema:                 "", // Dynamic - provided in request
		Version:                "20250116000000",
		Name:                   "create_user_authentication_and_authorization_system_with_role_based_access_control",
		Connection:             "core",
		Backend:                "postgresql",
		UpSQL:                  upSQL,
		DownSQL:                downSQL,
		Dependencies:           []string{},
		StructuredDependencies: []migrations.Dependency{},
	}
	migrations.GlobalRegistry.Register(migration)
}
