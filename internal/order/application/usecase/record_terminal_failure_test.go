package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"eventflow/internal/order/application/event"
	"eventflow/internal/order/application/port"
)

func TestRecordTerminalFailureGuardaCuartoFalloYSanitiza(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	repo := &terminalRepositoryDouble{}
	useCase := NewRecordTerminalFailureUseCase(repo, func() time.Time { return now })
	envelope := event.NewOrderCreated("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "correlation", now)
	if err := useCase.Execute(context.Background(), "orders-worker", envelope, 4, errors.New("password=secreto")); err != nil {
		t.Fatalf("registrar terminal: %v", err)
	}
	if repo.terminal.Attempt != 4 || repo.terminal.Failure != terminalFailureReason || !repo.terminal.FailedAt.Equal(now) {
		t.Fatalf("entrada terminal inesperada: %+v", repo.terminal)
	}
	if err := useCase.Execute(context.Background(), "orders-worker", envelope, 3, errors.New("fallo")); err == nil {
		t.Fatal("un intento no terminal no debe registrarse")
	}
}

func TestRecordTerminalFailureRechazaSinConservarBody(t *testing.T) {
	repo := &terminalRepositoryDouble{}
	useCase := NewRecordTerminalFailureUseCase(repo, fixedNow)
	if err := useCase.Reject(context.Background(), "orders-worker", []byte("secreto-no-persistible")); err != nil {
		t.Fatalf("registrar rechazo: %v", err)
	}
	if repo.rejected.PayloadDigest != "9e317238aadfe535a9c70db6758436201ef3b71a98d1a0149313e39902b9a751" || repo.rejected.Reason != "envelope invalido" {
		t.Fatalf("rechazo inesperado: %+v", repo.rejected)
	}
}

type terminalRepositoryDouble struct {
	terminal port.TerminalFailure
	rejected port.RejectedDelivery
}

func (d *terminalRepositoryDouble) RecordTerminalFailure(_ context.Context, input port.TerminalFailure) error {
	d.terminal = input
	return nil
}

func (d *terminalRepositoryDouble) RecordRejectedDelivery(_ context.Context, input port.RejectedDelivery) error {
	d.rejected = input
	return nil
}
