// Package migrations embeds database migration files.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
