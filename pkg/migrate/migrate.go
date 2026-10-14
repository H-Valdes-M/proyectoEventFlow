package migrate

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"eventflow/pkg/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

var migrationName = regexp.MustCompile(`^(\d{3})_([a-z0-9_]+)\.up\.sql$`)

// Result resume las migraciones aplicadas y omitidas durante una ejecución.
type Result struct {
	Applied int
	Skipped int
}

type migration struct {
	version int64
	name    string
	sql     string
}

// Apply ejecuta las migraciones SQL en orden y marca cada versión solo tras
// confirmar la transacción que la aplicó.
func Apply(ctx context.Context, source fs.FS, provider database.Database) (Result, error) {
	pool, err := database.AsPostgresPool(provider)
	if err != nil {
		return Result{}, err
	}
	if _, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS eventflow_schema_migrations (
    version BIGINT PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`); err != nil {
		return Result{}, fmt.Errorf("preparar registro de migraciones: %w", err)
	}

	migrations, err := load(source)
	if err != nil {
		return Result{}, err
	}
	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return Result{}, fmt.Errorf("leer registro de migraciones: %w", err)
	}

	var result Result
	for _, current := range migrations {
		if applied[current.version] {
			result.Skipped++
			continue
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return result, fmt.Errorf("iniciar migracion %03d: %w", current.version, err)
		}
		if _, err := tx.Exec(ctx, current.sql); err != nil {
			_ = tx.Rollback(ctx)
			return result, fmt.Errorf("aplicar migracion %03d", current.version)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO eventflow_schema_migrations (version, name) VALUES ($1, $2)`, current.version, current.name); err != nil {
			_ = tx.Rollback(ctx)
			return result, fmt.Errorf("registrar migracion %03d", current.version)
		}
		if err := tx.Commit(ctx); err != nil {
			return result, fmt.Errorf("confirmar migracion %03d", current.version)
		}
		result.Applied++
	}
	return result, nil
}

func load(source fs.FS) ([]migration, error) {
	files, err := fs.Glob(source, "*.up.sql")
	if err != nil {
		return nil, fmt.Errorf("listar migraciones: %w", err)
	}
	migrations := make([]migration, 0, len(files))
	versions := make(map[int64]struct{}, len(files))
	for _, file := range files {
		base := path.Base(file)
		match := migrationName.FindStringSubmatch(base)
		if match == nil {
			return nil, fmt.Errorf("nombre de migracion invalido: %s", base)
		}
		version, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("version de migracion invalida: %s", base)
		}
		if _, exists := versions[version]; exists {
			return nil, fmt.Errorf("version de migracion repetida: %03d", version)
		}
		sql, err := fs.ReadFile(source, file)
		if err != nil {
			return nil, fmt.Errorf("leer migracion %s: %w", base, err)
		}
		if strings.TrimSpace(string(sql)) == "" {
			return nil, fmt.Errorf("migracion vacia: %s", base)
		}
		versions[version] = struct{}{}
		migrations = append(migrations, migration{version: version, name: base, sql: string(sql)})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	return migrations, nil
}

func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[int64]bool, error) {
	rows, err := pool.Query(ctx, `SELECT version FROM eventflow_schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := make(map[int64]bool)
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions[version] = true
	}
	return versions, rows.Err()
}
