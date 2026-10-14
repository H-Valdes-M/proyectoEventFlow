package migrations

import "embed"

// Files contiene las migraciones SQL distribuidas junto al binario migrador.
//
//go:embed *.up.sql
var Files embed.FS
