package database

import "context"

// Database abstrae las operaciones de ciclo de vida comunes de una base de
// datos. Native solo se usa en adaptadores de infraestructura autorizados.
type Database interface {
	Ping(context.Context) error
	Close()
	Native() any
}
