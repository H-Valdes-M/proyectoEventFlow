package database

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresConfig contiene solo los datos necesarios para crear el pool. No se
// debe registrar ni devolver como parte de mensajes de error públicos.
type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

type postgresPool interface {
	Ping(context.Context) error
	Close()
}

// PostgresDatabase implementa Database sobre un pool de pgx.
type PostgresDatabase struct {
	pool   postgresPool
	native *pgxpool.Pool
}

// NewPostgresDatabase crea un pool sin afirmar conectividad. La disponibilidad
// se comprueba más tarde mediante Ping con el contexto y plazo del llamador.
func NewPostgresDatabase(ctx context.Context, config PostgresConfig) (*PostgresDatabase, error) {
	pool, err := pgxpool.New(ctx, postgresDSN(config))
	if err != nil {
		return nil, errors.New("configuracion de PostgreSQL invalida")
	}
	return &PostgresDatabase{pool: pool, native: pool}, nil
}

// Ping comprueba conectividad sin ocultar el contexto de cancelación del
// llamador.
func (d *PostgresDatabase) Ping(ctx context.Context) error {
	if d == nil || d.pool == nil {
		return errors.New("provider PostgreSQL no inicializado")
	}
	return d.pool.Ping(ctx)
}

// Close libera el pool. Es seguro para instancias no inicializadas.
func (d *PostgresDatabase) Close() {
	if d == nil || d.pool == nil {
		return
	}
	d.pool.Close()
}

// Native expone el pool real solo para adaptadores de infraestructura.
func (d *PostgresDatabase) Native() any {
	if d == nil {
		return nil
	}
	return d.native
}

// AsPostgresPool obtiene el pool concreto o devuelve un error controlado para
// impedir assertions que puedan causar panic en infraestructura.
func AsPostgresPool(database Database) (*pgxpool.Pool, error) {
	if database == nil {
		return nil, errors.New("provider de base de datos no configurado")
	}
	pool, ok := database.Native().(*pgxpool.Pool)
	if !ok || pool == nil {
		return nil, errors.New("provider de base de datos incompatible con PostgreSQL")
	}
	return pool, nil
}

func postgresDSN(config PostgresConfig) string {
	connection := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(config.User, config.Password),
		Host:   net.JoinHostPort(config.Host, strconv.Itoa(config.Port)),
		Path:   config.Database,
	}
	query := connection.Query()
	query.Set("sslmode", config.SSLMode)
	connection.RawQuery = query.Encode()
	return connection.String()
}
