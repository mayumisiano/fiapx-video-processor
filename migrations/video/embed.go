// Package videomigrations embeds video-api/video-worker's own migrations
// into their binaries, so each can apply them on startup without a
// separate init container or a volume-mounted SQL directory
// (docs/adr/0009).
package videomigrations

import "embed"

//go:embed *.sql
var FS embed.FS
