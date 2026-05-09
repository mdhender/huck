// Package web embeds the templates and static assets shipped with the binary.
package web

import "embed"

// Templates contains every template under web/templates.
//
//go:embed templates
var Templates embed.FS

// Static contains every static asset under web/static.
//
//go:embed static
var Static embed.FS
