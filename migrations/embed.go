package migrations

import "embed"

// FS contains SQL migrations bundled with the binary.
//
//go:embed *.sql
var FS embed.FS
