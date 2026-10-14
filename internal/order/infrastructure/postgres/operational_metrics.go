package postgres

import (
	"context"

	"eventflow/pkg/database"
	"eventflow/pkg/metrics"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OperationalMetrics consulta agregados sin convertir identificadores en etiquetas.
type OperationalMetrics struct{ pool *pgxpool.Pool }

func NewOperationalMetrics(provider database.Database) (*OperationalMetrics, error) {
	pool, err := database.AsPostgresPool(provider)
	if err != nil {
		return nil, err
	}
	return &OperationalMetrics{pool: pool}, nil
}

func (m *OperationalMetrics) Read(ctx context.Context) (metrics.DurableStats, error) {
	var result metrics.DurableStats
	err := m.pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM outbox_events WHERE published_at IS NULL),
  COALESCE((SELECT sum(attempts) FROM event_processing), 0),
  (SELECT count(*) FROM event_processing WHERE status = 'completed'),
  (SELECT count(*) FROM event_processing WHERE status = 'failed')`).Scan(
		&result.OutboxPending, &result.ProcessingAttempts, &result.Processed, &result.TerminalFailures)
	return result, err
}
