package usecase

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"eventflow/internal/order/application/event"
	"eventflow/internal/order/application/port"
)

func TestScheduleRetryProgramaDemorasEIDsInyectados(t *testing.T) {
	now := time.Date(2026, 10, 13, 22, 0, 0, 0, time.UTC)
	repo := &retryRepositoryDouble{}
	useCase := NewScheduleRetryUseCase(repo, func() time.Time { return now }, func() (string, error) { return "33333333-3333-4333-8333-333333333333", nil })
	envelope := event.NewOrderCreated("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "correlation", now)
	cause := fmt.Errorf("%w: password=secret-value", port.ErrTransientProcessing)

	for attempt, delay := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second} {
		if _, err := useCase.Execute(context.Background(), "orders-worker", envelope, attempt, cause); err != nil {
			t.Fatalf("programar intento %d: %v", attempt, err)
		}
		input := repo.inputs[len(repo.inputs)-1]
		if input.MessageID == "" || input.Attempt != attempt || !input.NextAttempt.Equal(now.Add(delay)) || input.CorrelationID != "correlation" || input.Failure != port.ErrTransientProcessing.Error() {
			t.Fatalf("programacion inesperada: %+v", input)
		}
	}
}

func TestScheduleRetryNoProgramaCuartoIntento(t *testing.T) {
	repo := &retryRepositoryDouble{}
	useCase := NewScheduleRetryUseCase(repo, fixedNow, func() (string, error) { return "id", nil })
	_, err := useCase.Execute(context.Background(), "orders-worker", event.Envelope{}, 4, errors.New("fallo"))
	if err == nil || len(repo.inputs) != 0 {
		t.Fatalf("el cuarto intento pertenece al fallo terminal: err=%v inputs=%d", err, len(repo.inputs))
	}
}

type retryRepositoryDouble struct {
	inputs []port.ScheduleRetry
	err    error
}

func (d *retryRepositoryDouble) ScheduleRetry(_ context.Context, input port.ScheduleRetry) (port.ScheduleRetryResult, error) {
	d.inputs = append(d.inputs, input)
	return port.ScheduleRetryResult{}, d.err
}
