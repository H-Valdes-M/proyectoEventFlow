package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"eventflow/cmd/config"
	postgreshealth "eventflow/internal/health/infrastructure/postgres"
	"eventflow/pkg/database"
)

func TestReadinessIntegracionConEsquemaActual(t *testing.T) {
	if os.Getenv("EVENTFLOW_PG_INTEGRATION") != "1" {
		t.Skip("definir EVENTFLOW_PG_INTEGRATION=1 para ejecutar contra PostgreSQL real")
	}
	t.Chdir(filepath.Join("..", "..", "..", ".."))
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal("cargar configuracion")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	db, err := database.NewPostgresDatabase(ctx, database.PostgresConfig{Host: cfg.DBHost, Port: cfg.DBPort, User: cfg.DBUser, Password: cfg.DBPassword, Database: cfg.DBName, SSLMode: cfg.DBSSLMode})
	if err != nil {
		t.Fatal("crear provider")
	}
	defer db.Close()
	checker, err := postgreshealth.NewReadinessChecker(db)
	if err != nil {
		t.Fatal("crear readiness")
	}
	if err := checker.Ready(ctx); err != nil {
		t.Fatalf("readiness debe aceptar DB y esquema actual: %v", err)
	}
}
