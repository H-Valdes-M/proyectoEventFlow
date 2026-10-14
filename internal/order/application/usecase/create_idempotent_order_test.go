package usecase

import (
	"context"
	"errors"
	"testing"

	"eventflow/internal/order/application/port"
	"eventflow/pkg/apperrors"
	"eventflow/pkg/correlation"
)

func TestCreateIdempotentOrderRepiteElSnapshotOriginal(t *testing.T) {
	repository := &idempotentRepositoryDouble{}
	useCase := NewCreateIdempotentOrderUseCase(repository, fixedNow, fixedID)

	first, err := useCase.Execute(context.Background(), validInput(), "pedido-1")
	if err != nil {
		t.Fatalf("primera creacion: %v", err)
	}
	second, err := useCase.Execute(context.Background(), validInput(), "pedido-1")
	if err != nil {
		t.Fatalf("repeticion: %v", err)
	}
	if first != second || first.ID != fixedOrderID || len(repository.inputs) != 2 || repository.inputs[0].Fingerprint != repository.inputs[1].Fingerprint {
		t.Fatalf("replay inesperado: first=%+v second=%+v inputs=%+v", first, second, repository.inputs)
	}
}

func TestCreateIdempotentOrderConservaCorrelacionDelContextoEnEvento(t *testing.T) {
	repository := &idempotentRepositoryDouble{}
	useCase := NewCreateIdempotentOrderUseCase(repository, fixedNow, fixedID)
	_, err := useCase.Execute(correlation.WithID(context.Background(), "req-demo_1"), validInput(), "pedido-1")
	if err != nil || len(repository.inputs) != 1 || repository.inputs[0].Event.CorrelationID != "req-demo_1" {
		t.Fatalf("correlacion del evento inesperada: err=%v inputs=%+v", err, repository.inputs)
	}
}

func TestCreateIdempotentOrderTraduceConflictoDeClave(t *testing.T) {
	repository := &idempotentRepositoryDouble{err: port.ErrIdempotencyConflict}
	useCase := NewCreateIdempotentOrderUseCase(repository, fixedNow, fixedID)

	_, err := useCase.Execute(context.Background(), validInput(), "pedido-1")
	if err == nil || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("conflicto inesperado: %v", err)
	}
}

type idempotentRepositoryDouble struct {
	inputs []port.IdempotentOrder
	err    error
}

func (d *idempotentRepositoryDouble) CreateIdempotent(_ context.Context, input port.IdempotentOrder) (port.IdempotentOrderResult, error) {
	d.inputs = append(d.inputs, input)
	if d.err != nil {
		return port.IdempotentOrderResult{}, d.err
	}
	return port.IdempotentOrderResult{Output: d.inputs[0].Snapshot, Replay: len(d.inputs) > 1}, nil
}
