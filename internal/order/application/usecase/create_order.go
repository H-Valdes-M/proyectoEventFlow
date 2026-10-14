package usecase

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"eventflow/internal/order/application/dto"
	"eventflow/internal/order/domain/entity"
	"eventflow/internal/order/domain/repository"
	"eventflow/pkg/apperrors"
)

// CreateOrderUseCase coordina la validación de dominio y la persistencia.
type CreateOrderUseCase struct {
	repository repository.OrderRepository
	now        func() time.Time
	newID      func() (string, error)
}

// NewCreateOrderUseCase construye el caso de uso con sus dependencias explícitas.
func NewCreateOrderUseCase(repository repository.OrderRepository, now func() time.Time, newID func() (string, error)) *CreateOrderUseCase {
	return &CreateOrderUseCase{repository: repository, now: now, newID: newID}
}

// Execute crea y guarda un pedido validado.
func (u *CreateOrderUseCase) Execute(ctx context.Context, input dto.CreateOrderInput) (dto.CreateOrderOutput, error) {
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
	if err := u.repository.Save(ctx, order); err != nil {
		return dto.CreateOrderOutput{}, fmt.Errorf("guardar pedido: %w", err)
	}
	return dto.CreateOrderOutput{ID: order.ID, Status: string(order.Status), TotalMinor: order.TotalMinor}, nil
}

// NewUUID genera un UUID v4 compatible con la columna orders.id.
func NewUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
