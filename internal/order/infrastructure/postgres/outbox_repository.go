package postgres

import (
	"context"
	"errors"
	"time"

	"eventflow/internal/order/application/port"
	"eventflow/pkg/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxRepository lee y confirma el estado de publicación de eventos.
type OutboxRepository struct {
	pool *pgxpool.Pool
}

// NewOutboxRepository construye el adaptador PostgreSQL para el relay.
func NewOutboxRepository(provider database.Database) (*OutboxRepository, error) {
	pool, err := database.AsPostgresPool(provider)
	if err != nil {
		return nil, err
	}
	return &OutboxRepository{pool: pool}, nil
}

// ReadReady devuelve solo mensajes aún no publicados cuya fecha ya llegó.
func (r *OutboxRepository) ReadReady(ctx context.Context, now time.Time, limit int) ([]port.OutboxEvent, error) {
	if limit < 1 {
		return nil, errors.New("el limite del outbox debe ser positivo")
	}
	rows, err := r.pool.Query(ctx, `
SELECT message_id, event_id, event_type, attempt, payload
FROM outbox_events
WHERE published_at IS NULL AND available_at <= $1
ORDER BY available_at, message_id
LIMIT $2`, now.UTC(), limit)
	if err != nil {
		return nil, persistenceError(err)
	}
	defer rows.Close()
	events := make([]port.OutboxEvent, 0)
	for rows.Next() {
		var event port.OutboxEvent
		if err := rows.Scan(&event.MessageID, &event.EventID, &event.Type, &event.Attempt, &event.Payload); err != nil {
			return nil, persistenceError(err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, persistenceError(err)
	}
	return events, nil
}

// MarkPublished cambia una fila solo después de una confirmación positiva.
func (r *OutboxRepository) MarkPublished(ctx context.Context, messageID string, at time.Time) error {
	command, err := r.pool.Exec(ctx, `
UPDATE outbox_events
SET published_at = $2, publish_attempts = publish_attempts + 1, last_publish_error = NULL
WHERE message_id = $1 AND published_at IS NULL`, messageID, at.UTC())
	if err != nil {
		return persistenceError(err)
	}
	if command.RowsAffected() != 1 {
		return errors.New("evento outbox no disponible para marcar publicado")
	}
	return nil
}
