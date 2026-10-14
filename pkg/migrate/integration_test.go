package migrate_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"eventflow/cmd/config"
	"eventflow/migrations"
	"eventflow/pkg/database"
	"eventflow/pkg/migrate"
)

func TestApplyIntegracionEsIdempotenteYNoRegistraFallo(t *testing.T) {
	if os.Getenv("EVENTFLOW_PG_INTEGRATION") != "1" {
		t.Skip("definir EVENTFLOW_PG_INTEGRATION=1 para ejecutar contra PostgreSQL real")
	}
	t.Chdir(filepath.Join("..", ".."))
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal("cargar configuracion de integración")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	db, err := database.NewPostgresDatabase(ctx, database.PostgresConfig{
		Host:     cfg.DBHost,
		Port:     cfg.DBPort,
		User:     cfg.DBUser,
		Password: cfg.DBPassword,
		Database: cfg.DBName,
		SSLMode:  cfg.DBSSLMode,
	})
	if err != nil {
		t.Fatal("crear provider PostgreSQL para integración")
	}
	defer db.Close()

	first, err := migrate.Apply(ctx, migrations.Files, db)
	if err != nil || first.Applied > 1 {
		t.Fatal("aplicar migracion técnica")
	}
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		t.Fatal("listar migraciones embebidas")
	}
	expectedSkipped := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".up.sql") {
			expectedSkipped++
		}
	}
	second, err := migrate.Apply(ctx, migrations.Files, db)
	if err != nil || second.Applied != 0 || second.Skipped != expectedSkipped {
		t.Fatal("la segunda ejecución debe omitir la migracion ya aplicada")
	}
	assertSchemaPedidos(t, ctx, db)
	failed, err := migrate.Apply(ctx, fstest.MapFS{
		"999_invalid.up.sql": {Data: []byte("SELECT funcion_que_no_existe();")},
	}, db)
	if err == nil || failed.Applied != 0 {
		t.Fatal("una migracion SQL invalida debe fallar sin aplicarse")
	}
	pool, err := database.AsPostgresPool(db)
	if err != nil {
		t.Fatal("obtener pool PostgreSQL")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM eventflow_schema_migrations WHERE version = 999`).Scan(&count); err != nil || count != 0 {
		t.Fatal("la version fallida no debe quedar registrada")
	}
}

func assertSchemaPedidos(t *testing.T, ctx context.Context, db database.Database) {
	t.Helper()
	pool, err := database.AsPostgresPool(db)
	if err != nil {
		t.Fatal("obtener pool PostgreSQL")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("iniciar transaccion de aceptación")
	}
	defer tx.Rollback(ctx)

	const orderID = "11111111-1111-1111-1111-111111111111"
	if _, err := tx.Exec(ctx, `
INSERT INTO orders (id, customer_id, total_minor, status, created_at, updated_at)
VALUES ($1, 'cliente-mt12', 3000, 'pending', NOW(), NOW())`, orderID); err != nil {
		t.Fatal("insertar pedido válido")
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO order_items (order_id, product_id, quantity, unit_price_minor)
VALUES ($1, 'producto-mt12', 2, 1500)`, orderID); err != nil {
		t.Fatal("insertar articulo válido")
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM order_items WHERE order_id = $1`, orderID).Scan(&count); err != nil || count != 1 {
		t.Fatal("la relacion pedido-articulos debe persistir")
	}
	invalidStatements := []string{
		`INSERT INTO orders (id, customer_id, total_minor, status, created_at, updated_at) VALUES ('22222222-2222-2222-2222-222222222222', 'cliente', 1, 'unknown', NOW(), NOW())`,
		`INSERT INTO order_items (order_id, product_id, quantity, unit_price_minor) VALUES ('33333333-3333-3333-3333-333333333333', 'sin-pedido', 1, 1)`,
		`INSERT INTO order_items (order_id, product_id, quantity, unit_price_minor) VALUES ('11111111-1111-1111-1111-111111111111', 'cantidad-invalida', 0, 1)`,
		`INSERT INTO order_items (order_id, product_id, quantity, unit_price_minor) VALUES ('11111111-1111-1111-1111-111111111111', 'precio-invalido', 1, 0)`,
		`INSERT INTO order_items (order_id, product_id, quantity, unit_price_minor) VALUES ('11111111-1111-1111-1111-111111111111', 'producto-mt12', 1, 1)`,
	}
	for _, statement := range invalidStatements {
		if _, err := tx.Exec(ctx, "SAVEPOINT invalid_input"); err != nil {
			t.Fatal("crear savepoint para restriccion")
		}
		if _, err := tx.Exec(ctx, statement); err == nil {
			t.Fatal("una restriccion del esquema debio rechazar el dato inválido")
		}
		if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT invalid_input"); err != nil {
			t.Fatal("restaurar transaccion tras restriccion")
		}
	}
}
