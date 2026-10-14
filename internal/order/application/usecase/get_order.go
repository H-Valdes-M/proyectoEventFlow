package usecase

import (
	"context"
	"errors"

	"eventflow/internal/order/application/dto"
	"eventflow/internal/order/domain/repository"
	"eventflow/pkg/apperrors"
)

// GetOrderUseCase coordina la lectura de un pedido sin modificarlo.
type GetOrderUseCase struct {
	repository repository.OrderRepository
}

// NewGetOrderUseCase construye el caso de uso de consulta.
func NewGetOrderUseCase(repository repository.OrderRepository) *GetOrderUseCase {
	return &GetOrderUseCase{repository: repository}
}

// Execute obtiene un pedido y lo mapea al DTO de salida.
func (u *GetOrderUseCase) Execute(ctx context.Context, id string) (dto.GetOrderOutput, error) {
	order, err := u.repository.FindByID(ctx, id)
	if errors.Is(err, repository.ErrOrderNotFound) {
		return dto.GetOrderOutput{}, apperrors.New(apperrors.NotFound, "ORDER_NOT_FOUND", "El pedido no existe")
	}
	if err != nil {
		return dto.GetOrderOutput{}, err
	}
	items := make([]dto.GetOrderItemOutput, len(order.Items))
	for index, item := range order.Items {
		items[index] = dto.GetOrderItemOutput{ProductID: item.ProductID, Quantity: item.Quantity, UnitPriceMinor: item.UnitPriceMinor}
	}
	return dto.GetOrderOutput{ID: order.ID, CustomerID: order.CustomerID, Items: items, TotalMinor: order.TotalMinor, Status: string(order.Status), CreatedAt: order.CreatedAt.UTC(), UpdatedAt: order.UpdatedAt.UTC()}, nil
}
