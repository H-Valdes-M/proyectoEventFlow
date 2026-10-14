package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"eventflow/internal/order/application/event"
	"eventflow/internal/order/application/port"
)

func TestProcessOrderEjecutaEfectoConDatosDelEvento(t *testing.T) {
	now := time.Date(2026, 10, 13, 20, 0, 0, 0, time.FixedZone("UTC-3", -3*60*60))
	repo := &processingRepositoryDouble{result: port.ProcessOrderResult{Duplicate: true}}
	useCase := NewProcessOrderUseCase(repo, func() time.Time { return now })
	ctx := context.WithValue(context.Background(), contextKey("request"), "value")
	envelope := event.NewOrderCreated("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "correlation", now)

	result, err := useCase.Execute(ctx, "orders-worker", envelope)
	if err != nil {
		t.Fatalf("ejecutar procesamiento: %v", err)
	}
	if !result.Duplicate || repo.context != ctx || repo.input.ConsumerName != "orders-worker" || repo.input.EventID != envelope.EventID || repo.input.OrderID != envelope.OrderID || !repo.input.ProcessedAt.Equal(now.UTC()) {
		t.Fatalf("llamada al repositorio inesperada: result=%+v input=%+v", result, repo.input)
	}
}

func TestProcessOrderPropagaFalloDePersistencia(t *testing.T) {
	failure := errors.New("conexion perdida")
	useCase := NewProcessOrderUseCase(&processingRepositoryDouble{err: failure}, fixedNow)

	_, err := useCase.Execute(context.Background(), "orders-worker", event.NewOrderCreated("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "correlation", fixedNow()))
	if !errors.Is(err, failure) {
		t.Fatalf("el fallo debe propagarse: %v", err)
	}
}

type processingRepositoryDouble struct {
	context context.Context
	input   port.ProcessOrder
	result  port.ProcessOrderResult
	err     error
}

func (d *processingRepositoryDouble) Process(ctx context.Context, input port.ProcessOrder) (port.ProcessOrderResult, error) {
	d.context = ctx
	d.input = input
	return d.result, d.err
}
