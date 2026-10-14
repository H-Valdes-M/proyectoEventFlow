package database

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestPostgresDatabasePingDelegaElContexto(t *testing.T) {
	want := errors.New("base no disponible")
	pool := &fakePostgresPool{pingErr: want}
	database := &PostgresDatabase{pool: pool}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := database.Ping(ctx); !errors.Is(err, want) {
		t.Fatalf("ping = %v, se esperaba %v", err, want)
	}
	if pool.context != ctx {
		t.Fatal("Ping debe propagar el contexto recibido")
	}
}

func TestPostgresDatabasePingSinProviderFallaControladamente(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (*PostgresDatabase)(nil).Ping(ctx); err == nil {
		t.Fatal("un provider no inicializado debe devolver error")
	}
}

func TestPostgresDatabaseCloseEsSeguro(t *testing.T) {
	pool := &fakePostgresPool{}
	database := &PostgresDatabase{pool: pool}
	database.Close()
	database.Close()
	if pool.closes != 2 {
		t.Fatalf("Close debe delegar para cada llamada, llego %d", pool.closes)
	}
	(*PostgresDatabase)(nil).Close()
}

func TestAsPostgresPoolRechazaProviderIncompatible(t *testing.T) {
	if _, err := AsPostgresPool(fakeDatabase{}); err == nil {
		t.Fatal("un provider incompatible debe devolver error")
	}
}

func TestPostgresDSNNoPierdeCaracteresDeConfiguracion(t *testing.T) {
	value, err := url.Parse(postgresDSN(PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "usuario@local",
		Password: "clave con espacio",
		Database: "eventflow",
		SSLMode:  "disable",
	}))
	if err != nil {
		t.Fatalf("parsear DSN: %v", err)
	}
	if value.User.Username() != "usuario@local" || value.Query().Get("sslmode") != "disable" {
		t.Fatalf("DSN inesperado: %s", value.Redacted())
	}
}

func TestPostgresPingIntegracion(t *testing.T) {
	if os.Getenv("EVENTFLOW_PG_INTEGRATION") != "1" {
		t.Skip("definir EVENTFLOW_PG_INTEGRATION=1 para ejecutar contra PostgreSQL real")
	}
	port, err := strconv.Atoi(os.Getenv("DB_PORT"))
	if err != nil {
		t.Fatal("DB_PORT debe ser un entero para la prueba de integración")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	database, err := NewPostgresDatabase(ctx, PostgresConfig{
		Host:     os.Getenv("DB_HOST"),
		Port:     port,
		User:     os.Getenv("DB_USER"),
		Password: os.Getenv("DB_PASSWORD"),
		Database: os.Getenv("DB_NAME"),
		SSLMode:  os.Getenv("DB_SSL_MODE"),
	})
	if err != nil {
		t.Fatal("crear provider PostgreSQL para integración")
	}
	defer database.Close()
	if err := database.Ping(ctx); err != nil {
		t.Fatal("Ping real contra PostgreSQL falló dentro del timeout")
	}
}

type fakePostgresPool struct {
	context context.Context
	pingErr error
	closes  int
}

func (p *fakePostgresPool) Ping(ctx context.Context) error {
	p.context = ctx
	return p.pingErr
}

func (p *fakePostgresPool) Close() {
	p.closes++
}

type fakeDatabase struct{}

func (fakeDatabase) Ping(context.Context) error { return nil }
func (fakeDatabase) Close()                     {}
func (fakeDatabase) Native() any                { return struct{}{} }
