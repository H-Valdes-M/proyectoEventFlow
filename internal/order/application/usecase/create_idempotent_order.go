package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"eventflow/internal/order/application/dto"
	"eventflow/internal/order/application/event"
	"eventflow/internal/order/application/port"
	"eventflow/internal/order/domain/entity"
	"eventflow/pkg/apperrors"
	"eventflow/pkg/correlation"
)

// CreateIdempotentOrderUseCase coordina una creación y su clave de idempotencia.
type CreateIdempotentOrderUseCase struct {
	repository port.IdempotentOrderRepository
	now        func() time.Time
	newID      func() (string, error)
}

// NewCreateIdempotentOrderUseCase construye el caso de uso con dependencias explícitas.
func NewCreateIdempotentOrderUseCase(repository port.IdempotentOrderRepository, now func() time.Time, newID func() (string, error)) *CreateIdempotentOrderUseCase {
	return &CreateIdempotentOrderUseCase{repository: repository, now: now, newID: newID}
}

// Execute crea el pedido o devuelve el snapshot original para la misma clave.
func (u *CreateIdempotentOrderUseCase) Execute(ctx context.Context, input dto.CreateOrderInput, key string) (dto.CreateOrderOutput, error) {
	fingerprint, err := CanonicalFingerprint(input)
	if err != nil {
		return dto.CreateOrderOutput{}, err
	}
	id, err := u.newID()
	if err != nil {
		return dto.CreateOrderOutput{}, apperrors.Wrap(err, apperrors.InternalError, "ORDER_ID_GENERATION", "No se pudo crear el pedido")
	}
	items := make([]entity.OrderItem, len(input.Items))
	for index, item := range input.Items {
		items[index] = entity.OrderItem{ProductID: item.ProductID, Quantity: item.Quantity, UnitPriceMinor: item.UnitPriceMinor}
	}
	order, err := entity.NewOrder(id, input.CustomerID, items, u.now())
	if err != nil {
		return dto.CreateOrderOutput{}, err
	}
	snapshot := dto.CreateOrderOutput{ID: order.ID, Status: string(order.Status), TotalMinor: order.TotalMinor}
	eventID, err := u.newID()
	if err != nil {
		return dto.CreateOrderOutput{}, apperrors.Wrap(err, apperrors.InternalError, "EVENT_ID_GENERATION", "No se pudo crear el evento del pedido")
	}
	messageID, err := u.newID()
	if err != nil {
		return dto.CreateOrderOutput{}, apperrors.Wrap(err, apperrors.InternalError, "MESSAGE_ID_GENERATION", "No se pudo crear el mensaje del evento")
	}
	correlationID := correlation.ID(ctx)
	if correlationID == "" {
		correlationID = order.ID
	}
	initialEvent := event.NewOrderCreated(eventID, order.ID, correlationID, order.CreatedAt)
	result, err := u.repository.CreateIdempotent(ctx, port.IdempotentOrder{Key: key, Fingerprint: fingerprint, Order: order, Snapshot: snapshot, Event: initialEvent, MessageID: messageID})
	if errors.Is(err, port.ErrIdempotencyConflict) {
		return dto.CreateOrderOutput{}, apperrors.New(apperrors.Conflict, "IDEMPOTENCY_KEY_CONFLICT", "La clave de idempotencia corresponde a otra solicitud")
	}
	if err != nil {
		return dto.CreateOrderOutput{}, fmt.Errorf("crear pedido idempotente: %w", err)
	}
	return result.Output, nil
}
