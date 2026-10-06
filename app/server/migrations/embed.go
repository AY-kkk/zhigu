package migrations

import "embed"

//go:embed finance/*.sql futures/*.sql
var FS embed.FS
