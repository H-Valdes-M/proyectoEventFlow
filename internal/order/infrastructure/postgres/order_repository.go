package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"eventflow/internal/order/application/dto"
	"eventflow/internal/order/application/event"
	"eventflow/internal/order/application/port"
	"eventflow/internal/order/domain/entity"
	"eventflow/internal/order/domain/repository"
	"eventflow/pkg/apperrors"
	"eventflow/pkg/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrderRepository persiste pedidos usando PostgreSQL.
type OrderRepository struct {
	pool *pgxpool.Pool
}

// NewOrderRepository construye el adaptador sobre el provider PostgreSQL.
func NewOrderRepository(provider database.Database) (*OrderRepository, error) {
	pool, err := database.AsPostgresPool(provider)
	if err != nil {
		return nil, err
	}
	return &OrderRepository{pool: pool}, nil
}

// Save guarda el pedido y todos sus artículos en la misma transacción.
func (r *OrderRepository) Save(ctx context.Context, order entity.Order) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return persistenceError(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
INSERT INTO orders (id, customer_id, total_minor, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)`, order.ID, order.CustomerID, order.TotalMinor, order.Status, order.CreatedAt, order.UpdatedAt); err != nil {
		return persistenceError(err)
	}
	for _, item := range order.Items {
		if _, err := tx.Exec(ctx, `
INSERT INTO order_items (order_id, product_id, quantity, unit_price_minor)
VALUES ($1, $2, $3, $4)`, order.ID, item.ProductID, item.Quantity, item.UnitPriceMinor); err != nil {
			return persistenceError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return persistenceError(err)
	}
	return nil
}

// CreateIdempotent reclama la clave y persiste pedido, artículos y snapshot en
// una sola transacción. La FK diferida permite reservar la clave antes del pedido.
func (r *OrderRepository) CreateIdempotent(ctx context.Context, input port.IdempotentOrder) (port.IdempotentOrderResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return port.IdempotentOrderResult{}, persistenceError(err)
	}
	defer tx.Rollback(ctx)
	snapshot, err := json.Marshal(input.Snapshot)
	if err != nil {
		return port.IdempotentOrderResult{}, persistenceError(err)
	}
	command, err := tx.Exec(ctx, `
INSERT INTO idempotency_keys (idempotency_key, fingerprint, order_id, response_status, response_body, created_at)
VALUES ($1, $2, $3, 201, $4, $5)
ON CONFLICT (idempotency_key) DO NOTHING`, input.Key, input.Fingerprint, input.Order.ID, snapshot, input.Order.CreatedAt)
	if err != nil {
		return port.IdempotentOrderResult{}, persistenceError(err)
	}
	if command.RowsAffected() == 0 {
		var fingerprint string
		var status int
		var body []byte
		if err := tx.QueryRow(ctx, `SELECT fingerprint, response_status, response_body FROM idempotency_keys WHERE idempotency_key = $1`, input.Key).Scan(&fingerprint, &status, &body); err != nil {
			return port.IdempotentOrderResult{}, persistenceError(err)
		}
		if fingerprint != input.Fingerprint {
			return port.IdempotentOrderResult{}, port.ErrIdempotencyConflict
		}
		var output dto.CreateOrderOutput
		if status != 201 || json.Unmarshal(body, &output) != nil {
			return port.IdempotentOrderResult{}, persistenceError(errors.New("snapshot de idempotencia invalido"))
		}
		return port.IdempotentOrderResult{Output: output, Replay: true}, nil
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO orders (id, customer_id, total_minor, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)`, input.Order.ID, input.Order.CustomerID, input.Order.TotalMinor, input.Order.Status, input.Order.CreatedAt, input.Order.UpdatedAt); err != nil {
		return port.IdempotentOrderResult{}, persistenceError(err)
	}
	for _, item := range input.Order.Items {
		if _, err := tx.Exec(ctx, `
INSERT INTO order_items (order_id, product_id, quantity, unit_price_minor)
VALUES ($1, $2, $3, $4)`, input.Order.ID, item.ProductID, item.Quantity, item.UnitPriceMinor); err != nil {
			return port.IdempotentOrderResult{}, persistenceError(err)
		}
	}
	payload, err := input.Event.Serialize()
	if err != nil {
		return port.IdempotentOrderResult{}, persistenceError(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO outbox_events (message_id, event_id, event_type, order_id, payload, available_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)`, input.MessageID, input.Event.EventID, input.Event.Type, input.Event.OrderID, payload, input.Event.OccurredAt, input.Order.CreatedAt); err != nil {
		return port.IdempotentOrderResult{}, persistenceError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return port.IdempotentOrderResult{}, persistenceError(err)
	}
	return port.IdempotentOrderResult{Output: input.Snapshot}, nil
}

// Process confirma el efecto simulado, el pedido procesado y el evento completado
// en una sola transacción. Un evento ya completado no repite el efecto.
func (r *OrderRepository) Process(ctx context.Context, input port.ProcessOrder) (port.ProcessOrderResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return port.ProcessOrderResult{}, persistenceError(err)
	}
	defer tx.Rollback(ctx)

	if input.Attempt < 1 {
		return port.ProcessOrderResult{}, persistenceError(errors.New("intento de procesamiento invalido"))
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO event_processing (consumer_name, event_id, order_id, status, attempts, created_at, updated_at)
VALUES ($1, $2, $3, 'ready', 1, $4, $4)
ON CONFLICT (consumer_name, event_id) DO NOTHING`, input.ConsumerName, input.EventID, input.OrderID, input.ProcessedAt); err != nil {
		return port.ProcessOrderResult{}, persistenceError(err)
	}

	var orderID, status string
	var attempts int
	var nextAttemptAt *time.Time
	if err := tx.QueryRow(ctx, `
SELECT order_id, status, attempts, next_attempt_at
FROM event_processing
WHERE consumer_name = $1 AND event_id = $2
FOR UPDATE`, input.ConsumerName, input.EventID).Scan(&orderID, &status, &attempts, &nextAttemptAt); err != nil {
		return port.ProcessOrderResult{}, persistenceError(err)
	}
	if orderID != input.OrderID || status == "completed" {
		if orderID != input.OrderID {
			return port.ProcessOrderResult{}, persistenceError(errors.New("evento asociado a otro pedido"))
		}
		return port.ProcessOrderResult{Duplicate: true}, nil
	}
	if status == "failed" {
		return port.ProcessOrderResult{Terminal: true}, nil
	}
	if status == "retry_scheduled" {
		if input.Attempt != attempts+1 {
			return port.ProcessOrderResult{RetryScheduled: true}, nil
		}
		if nextAttemptAt == nil || input.ProcessedAt.Before(*nextAttemptAt) {
			return port.ProcessOrderResult{RetryScheduled: true}, nil
		}
		if _, err := tx.Exec(ctx, `
UPDATE event_processing
SET status = 'ready', attempts = $3, next_attempt_at = NULL, updated_at = $4
WHERE consumer_name = $1 AND event_id = $2`, input.ConsumerName, input.EventID, input.Attempt, input.ProcessedAt); err != nil {
			return port.ProcessOrderResult{}, persistenceError(err)
		}
		status = "ready"
		attempts = input.Attempt
	}
	if status != "ready" || attempts != input.Attempt {
		return port.ProcessOrderResult{}, persistenceError(errors.New("evento no disponible para procesamiento"))
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO notifications (order_id, notification_type, created_at)
VALUES ($1, 'order.processed', $2)
ON CONFLICT (order_id, notification_type) DO NOTHING`, input.OrderID, input.ProcessedAt); err != nil {
		return port.ProcessOrderResult{}, persistenceError(err)
	}
	command, err := tx.Exec(ctx, `
UPDATE orders
SET status = 'processed', updated_at = CASE WHEN status = 'pending' THEN $2 ELSE updated_at END
WHERE id = $1 AND status IN ('pending', 'processed')`, input.OrderID, input.ProcessedAt)
	if err != nil {
		return port.ProcessOrderResult{}, persistenceError(err)
	}
	if command.RowsAffected() != 1 {
		return port.ProcessOrderResult{}, persistenceError(errors.New("pedido no disponible para procesamiento"))
	}
	if _, err := tx.Exec(ctx, `
UPDATE event_processing
SET status = 'completed', next_attempt_at = NULL, last_error_sanitized = NULL, updated_at = $3
WHERE consumer_name = $1 AND event_id = $2`, input.ConsumerName, input.EventID, input.ProcessedAt); err != nil {
		return port.ProcessOrderResult{}, persistenceError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return port.ProcessOrderResult{}, persistenceError(err)
	}
	return port.ProcessOrderResult{}, nil
}

// ScheduleRetry guarda la decisión durable y el próximo mensaje en una sola
// transacción. Las entregas antiguas no pueden adelantar ni duplicar el plan.
func (r *OrderRepository) ScheduleRetry(ctx context.Context, input port.ScheduleRetry) (port.ScheduleRetryResult, error) {
	if input.Attempt < 1 || input.Attempt >= 4 {
		return port.ScheduleRetryResult{}, persistenceError(errors.New("intento no programable"))
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return port.ScheduleRetryResult{}, persistenceError(err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
INSERT INTO event_processing (consumer_name, event_id, order_id, status, attempts, created_at, updated_at)
VALUES ($1, $2, $3, 'ready', 1, $4, $4)
ON CONFLICT (consumer_name, event_id) DO NOTHING`, input.ConsumerName, input.EventID, input.OrderID, input.ScheduledAt); err != nil {
		return port.ScheduleRetryResult{}, persistenceError(err)
	}
	var orderID, status string
	var attempts int
	if err := tx.QueryRow(ctx, `
SELECT order_id, status, attempts FROM event_processing
WHERE consumer_name = $1 AND event_id = $2 FOR UPDATE`, input.ConsumerName, input.EventID).Scan(&orderID, &status, &attempts); err != nil {
		return port.ScheduleRetryResult{}, persistenceError(err)
	}
	if orderID != input.OrderID {
		return port.ScheduleRetryResult{}, persistenceError(errors.New("evento asociado a otro pedido"))
	}
	if status == "completed" {
		return port.ScheduleRetryResult{Completed: true}, nil
	}
	if status == "retry_scheduled" && attempts == input.Attempt {
		return port.ScheduleRetryResult{AlreadyScheduled: true}, nil
	}
	if status == "retry_scheduled" && attempts == input.Attempt-1 {
		if _, err := tx.Exec(ctx, `
UPDATE event_processing
SET status = 'ready', attempts = $3, next_attempt_at = NULL, updated_at = $4
WHERE consumer_name = $1 AND event_id = $2`, input.ConsumerName, input.EventID, input.Attempt, input.ScheduledAt); err != nil {
			return port.ScheduleRetryResult{}, persistenceError(err)
		}
		status, attempts = "ready", input.Attempt
	}
	if status != "ready" || attempts != input.Attempt {
		return port.ScheduleRetryResult{AlreadyScheduled: true}, nil
	}
	payload, err := event.NewOrderCreated(input.EventID, input.OrderID, input.CorrelationID, input.ScheduledAt).Serialize()
	if err != nil {
		return port.ScheduleRetryResult{}, persistenceError(err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE event_processing
SET status = 'retry_scheduled', next_attempt_at = $3, last_error_sanitized = $4, updated_at = $5
WHERE consumer_name = $1 AND event_id = $2`, input.ConsumerName, input.EventID, input.NextAttempt, input.Failure, input.ScheduledAt); err != nil {
		return port.ScheduleRetryResult{}, persistenceError(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO outbox_events (message_id, event_id, event_type, order_id, payload, available_at, created_at, consumer_name, attempt)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, input.MessageID, input.EventID, event.OrderCreatedType, input.OrderID, payload, input.NextAttempt, input.ScheduledAt, input.ConsumerName, input.Attempt+1); err != nil {
		return port.ScheduleRetryResult{}, persistenceError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return port.ScheduleRetryResult{}, persistenceError(err)
	}
	return port.ScheduleRetryResult{}, nil
}

// RecordTerminalFailure marca el pedido, procesamiento y evidencia de investigación juntos.
func (r *OrderRepository) RecordTerminalFailure(ctx context.Context, input port.TerminalFailure) error {
	if input.Attempt != 4 {
		return persistenceError(errors.New("intento terminal invalido"))
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return persistenceError(err)
	}
	defer tx.Rollback(ctx)

	var orderID, status string
	var attempts int
	if err := tx.QueryRow(ctx, `
SELECT order_id, status, attempts FROM event_processing
WHERE consumer_name = $1 AND event_id = $2 FOR UPDATE`, input.ConsumerName, input.EventID).Scan(&orderID, &status, &attempts); err != nil {
		return persistenceError(err)
	}
	if orderID != input.OrderID {
		return persistenceError(errors.New("evento asociado a otro pedido"))
	}
	if status == "failed" {
		if err := tx.Commit(ctx); err != nil {
			return persistenceError(err)
		}
		return nil
	}
	if status == "retry_scheduled" && attempts == input.Attempt-1 {
		if _, err := tx.Exec(ctx, `
UPDATE event_processing
SET status = 'ready', attempts = $3, next_attempt_at = NULL, updated_at = $4
WHERE consumer_name = $1 AND event_id = $2`, input.ConsumerName, input.EventID, input.Attempt, input.FailedAt); err != nil {
			return persistenceError(err)
		}
		status, attempts = "ready", input.Attempt
	}
	if status != "ready" || attempts != input.Attempt {
		return persistenceError(errors.New("evento no disponible para fallo terminal"))
	}
	command, err := tx.Exec(ctx, `
UPDATE orders SET status = 'failed', updated_at = $2
WHERE id = $1 AND status = 'pending'`, input.OrderID, input.FailedAt)
	if err != nil {
		return persistenceError(err)
	}
	if command.RowsAffected() != 1 {
		return persistenceError(errors.New("pedido no disponible para fallo terminal"))
	}
	if _, err := tx.Exec(ctx, `
UPDATE event_processing
SET status = 'failed', next_attempt_at = NULL, last_error_sanitized = $3, updated_at = $4
WHERE consumer_name = $1 AND event_id = $2`, input.ConsumerName, input.EventID, input.Failure, input.FailedAt); err != nil {
		return persistenceError(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO failed_events (consumer_name, event_id, order_id, reason_sanitized, failed_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (consumer_name, event_id) DO NOTHING`, input.ConsumerName, input.EventID, input.OrderID, input.Failure, input.FailedAt); err != nil {
		return persistenceError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return persistenceError(err)
	}
	return nil
}

// RecordRejectedDelivery conserva una huella para investigar sin persistir el body.
func (r *OrderRepository) RecordRejectedDelivery(ctx context.Context, input port.RejectedDelivery) error {
	if len(input.PayloadDigest) != 64 || input.ConsumerName == "" {
		return persistenceError(errors.New("rechazo invalido"))
	}
	_, err := r.pool.Exec(ctx, `
INSERT INTO rejected_deliveries (consumer_name, payload_digest, reason_sanitized, rejected_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (consumer_name, payload_digest) DO NOTHING`, input.ConsumerName, input.PayloadDigest, input.Reason, input.RejectedAt)
	if err != nil {
		return persistenceError(err)
	}
	return nil
}

// FindByID reconstruye un pedido sin modificar la base de datos.
func (r *OrderRepository) FindByID(ctx context.Context, id string) (entity.Order, error) {
	var order entity.Order
	err := r.pool.QueryRow(ctx, `
SELECT id, customer_id, total_minor, status, created_at, updated_at
FROM orders WHERE id = $1`, id).Scan(
		&order.ID, &order.CustomerID, &order.TotalMinor, &order.Status, &order.CreatedAt, &order.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.Order{}, repository.ErrOrderNotFound
	}
	if err != nil {
		return entity.Order{}, persistenceError(err)
	}
	rows, err := r.pool.Query(ctx, `
SELECT product_id, quantity, unit_price_minor
FROM order_items WHERE order_id = $1 ORDER BY product_id`, id)
	if err != nil {
		return entity.Order{}, persistenceError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var item entity.OrderItem
		if err := rows.Scan(&item.ProductID, &item.Quantity, &item.UnitPriceMinor); err != nil {
			return entity.Order{}, persistenceError(err)
		}
		order.Items = append(order.Items, item)
	}
	if err := rows.Err(); err != nil {
		return entity.Order{}, persistenceError(err)
	}
	return order, nil
}

func persistenceError(cause error) error {
	return apperrors.Wrap(cause, apperrors.InternalError, "ORDER_PERSISTENCE", "No se pudo completar la operacion del pedido")
}

var _ repository.OrderRepository = (*OrderRepository)(nil)
var _ port.IdempotentOrderRepository = (*OrderRepository)(nil)
var _ port.ProcessingRepository = (*OrderRepository)(nil)
var _ port.RetryRepository = (*OrderRepository)(nil)
var _ port.TerminalRepository = (*OrderRepository)(nil)
