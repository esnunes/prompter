// internal/server/embed.go
package server

import "embed"

//go:embed pages/*.html pages/*/*.html hx/*.html static
var contentFS embed.FS
