// Package identitymigrations embeds identity-api's own migrations into its
// binary, so it can apply them on startup without a separate init
// container or a volume-mounted SQL directory (docs/adr/0009).
package identitymigrations

import "embed"

//go:embed *.sql
var FS embed.FS
