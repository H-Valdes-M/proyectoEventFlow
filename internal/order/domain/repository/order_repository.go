package repository

import (
	"context"
	"errors"

	"eventflow/internal/order/domain/entity"
)

// ErrOrderNotFound distingue la ausencia de un pedido de un fallo operativo.
var ErrOrderNotFound = errors.New("pedido no encontrado")

// OrderRepository define la persistencia necesaria por los casos de uso.
type OrderRepository interface {
	Save(context.Context, entity.Order) error
	FindByID(context.Context, string) (entity.Order, error)
}
