package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"eventflow/internal/order/domain/entity"
	"eventflow/internal/order/domain/repository"
	"eventflow/pkg/apperrors"
)

func TestGetOrderMapeaPedidoSinModificarlo(t *testing.T) {
	now := time.Date(2026, 10, 13, 20, 0, 0, 0, time.FixedZone("UTC-3", -3*60*60))
	repo := &getRepositoryDouble{order: entity.Order{ID: fixedOrderID, CustomerID: "cliente-001", TotalMinor: 3000, Status: entity.OrderPending, CreatedAt: now, UpdatedAt: now, Items: []entity.OrderItem{{ProductID: "producto-001", Quantity: 2, UnitPriceMinor: 1500}}}}
	ctx := context.WithValue(context.Background(), contextKey("request"), "value")
	output, err := NewGetOrderUseCase(repo).Execute(ctx, fixedOrderID)
	if err != nil || repo.context != ctx || output.ID != fixedOrderID || output.CreatedAt.Location() != time.UTC || len(output.Items) != 1 {
		t.Fatalf("resultado inesperado: output=%+v err=%v", output, err)
	}
}

func TestGetOrderDistingueAusenciaYFallo(t *testing.T) {
	_, err := NewGetOrderUseCase(&getRepositoryDouble{err: repository.ErrOrderNotFound}).Execute(context.Background(), fixedOrderID)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("ausencia inesperada: %v", err)
	}
	operational := apperrors.Wrap(errors.New("conexion perdida"), apperrors.InternalError, "ORDER_PERSISTENCE", "No disponible")
	_, err = NewGetOrderUseCase(&getRepositoryDouble{err: operational}).Execute(context.Background(), fixedOrderID)
	if !errors.Is(err, apperrors.ErrInternal) {
		t.Fatalf("fallo operativo inesperado: %v", err)
	}
}

type getRepositoryDouble struct {
	order   entity.Order
	err     error
	context context.Context
}

func (d *getRepositoryDouble) Save(context.Context, entity.Order) error { return nil }

func (d *getRepositoryDouble) FindByID(ctx context.Context, _ string) (entity.Order, error) {
	d.context = ctx
	return d.order, d.err
}
