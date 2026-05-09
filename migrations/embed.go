// Package migrations holds the embedded SQL migration files used by
// internal/db.
package migrations

import "embed"

// FS holds every NNNN_*.sql file in this directory, in lexical order.
//
//go:embed *.sql
var FS embed.FS
