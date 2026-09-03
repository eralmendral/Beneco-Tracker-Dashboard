package migrations

import "embed"

// Files contains every versioned database migration.
//
//go:embed *.sql
var Files embed.FS
