// Package migrations embeds the SQL migration files so the server can apply
// them at boot without shipping a separate directory alongside the binary.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
