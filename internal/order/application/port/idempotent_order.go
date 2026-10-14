package port

import (
	"context"
	"errors"

	"eventflow/internal/order/application/dto"
	"eventflow/internal/order/application/event"
	"eventflow/internal/order/domain/entity"
)

// ErrIdempotencyConflict indica que una clave ya representa otra solicitud.
var ErrIdempotencyConflict = errors.New("conflicto de clave de idempotencia")

// IdempotentOrder agrupa los datos que deben persistirse en una sola transacción.
type IdempotentOrder struct {
	Key         string
	Fingerprint string
	Order       entity.Order
	Snapshot    dto.CreateOrderOutput
	Event       event.Envelope
	MessageID   string
}

// IdempotentOrderResult distingue una creación de la reproducción del snapshot.
type IdempotentOrderResult struct {
	Output dto.CreateOrderOutput
	Replay bool
}

// IdempotentOrderRepository persiste o recupera una creación idempotente.
type IdempotentOrderRepository interface {
	CreateIdempotent(context.Context, IdempotentOrder) (IdempotentOrderResult, error)
}
