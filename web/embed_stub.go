//go:build !embedui

// Package web embeds the production frontend build when compiled with
// -tags embedui. Without the tag the binary serves the API only, which keeps
// `go build ./...` and `go test ./...` working without Node.js.
package web

import "io/fs"

// FS returns nil: the UI is not embedded in this build.
func FS() fs.FS { return nil }
