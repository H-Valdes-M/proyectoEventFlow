// Package messaging conecta el estado outbox de pedidos con el transporte.
package messaging

import (
	"context"
	"time"

	orderevent "eventflow/internal/order/application/event"
	"eventflow/internal/order/application/port"
	"eventflow/pkg/pubsub"
	"go.uber.org/zap"
)

// Relay publica eventos pendientes sin asumir ejecución multiinstancia.
type Relay struct {
	repository port.OutboxRepository
	publisher  pubsub.Publisher
	now        func() time.Time
	log        *zap.Logger
}

// NewRelay ensambla las dos dependencias del relay para que el worker futuro
// controle el ciclo de ejecución.
func NewRelay(repository port.OutboxRepository, publisher pubsub.Publisher, now func() time.Time) *Relay {
	return &Relay{repository: repository, publisher: publisher, now: now, log: zap.NewNop()}
}

// WithLogger añade campos operativos seguros al relay del worker.
func (r *Relay) WithLogger(log *zap.Logger) *Relay {
	if log != nil {
		r.log = log
	}
	return r
}

// PublishPendingBatch publica hasta limit eventos listos. El primer error se
// devuelve sin marcar su fila, por lo que queda recuperable con el mismo ID.
func (r *Relay) PublishPendingBatch(ctx context.Context, limit int) error {
	events, err := r.repository.ReadReady(ctx, r.now(), limit)
	if err != nil {
		return err
	}
	for _, event := range events {
		correlationID, orderID := "", ""
		if envelope, err := orderevent.Deserialize(event.Payload); err == nil {
			correlationID, orderID = envelope.CorrelationID, envelope.OrderID
		}
		r.log.Info("publicar evento outbox", zap.String("correlation_id", correlationID), zap.String("order_id", orderID), zap.String("event_id", event.EventID), zap.Int("attempt", event.Attempt))
		if err := r.publisher.PublishConfirmed(ctx, pubsub.Message{
			MessageID: event.MessageID,
			EventID:   event.EventID,
			Type:      event.Type,
			Attempt:   event.Attempt,
			Payload:   event.Payload,
		}); err != nil {
			return err
		}
		if err := r.repository.MarkPublished(ctx, event.MessageID, r.now()); err != nil {
			return err
		}
	}
	return nil
}
