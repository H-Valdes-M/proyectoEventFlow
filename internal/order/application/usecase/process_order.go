package usecase

import (
	"context"
	"time"

	"eventflow/internal/order/application/event"
	"eventflow/internal/order/application/port"
)

// ProcessOrderUseCase coordina un efecto durable sin conocer el transporte AMQP.
type ProcessOrderUseCase struct {
	repository port.ProcessingRepository
	now        func() time.Time
}

// NewProcessOrderUseCase construye el caso de uso con sus dependencias explícitas.
func NewProcessOrderUseCase(repository port.ProcessingRepository, now func() time.Time) *ProcessOrderUseCase {
	return &ProcessOrderUseCase{repository: repository, now: now}
}

// Execute registra el efecto del pedido o informa que el evento ya fue completado.
func (u *ProcessOrderUseCase) Execute(ctx context.Context, consumerName string, envelope event.Envelope) (port.ProcessOrderResult, error) {
	return u.ExecuteAttempt(ctx, consumerName, envelope, 1)
}

// ExecuteAttempt procesa un intento de transporte validado por el adaptador.
func (u *ProcessOrderUseCase) ExecuteAttempt(ctx context.Context, consumerName string, envelope event.Envelope, attempt int) (port.ProcessOrderResult, error) {
	return u.repository.Process(ctx, port.ProcessOrder{
		ConsumerName: consumerName,
		EventID:      envelope.EventID,
		OrderID:      envelope.OrderID,
		Attempt:      attempt,
		ProcessedAt:  u.now().UTC(),
	})
}
