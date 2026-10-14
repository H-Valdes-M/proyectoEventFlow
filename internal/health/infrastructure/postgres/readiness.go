package postgres

import (
	"context"
	"fmt"

	"eventflow/pkg/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

const requiredMigrationVersion = 2

// ReadinessChecker comprueba PostgreSQL y las migraciones requeridas.
type ReadinessChecker struct {
	database database.Database
	pool     *pgxpool.Pool
}

// NewReadinessChecker construye la sonda de PostgreSQL.
func NewReadinessChecker(provider database.Database) (*ReadinessChecker, error) {
	pool, err := database.AsPostgresPool(provider)
	if err != nil {
		return nil, err
	}
	return &ReadinessChecker{database: provider, pool: pool}, nil
}

// Ready comprueba conectividad y que el esquema alcance la versión requerida.
func (r *ReadinessChecker) Ready(ctx context.Context) error {
	if err := r.database.Ping(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	var version int64
	if err := r.pool.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM eventflow_schema_migrations`).Scan(&version); err != nil {
		return fmt.Errorf("leer migraciones: %w", err)
	}
	if version < requiredMigrationVersion {
		return fmt.Errorf("migraciones insuficientes")
	}
	return nil
}
