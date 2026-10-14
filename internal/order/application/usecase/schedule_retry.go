package usecase

import (
	"context"
	"errors"
	"time"

	"eventflow/internal/order/application/event"
	"eventflow/internal/order/application/port"
)

// ScheduleRetryUseCase persiste la continuación de un fallo transitorio.
type ScheduleRetryUseCase struct {
	repository port.RetryRepository
	now        func() time.Time
	newID      func() (string, error)
}

// NewScheduleRetryUseCase construye el caso de uso con reloj e ID inyectables.
func NewScheduleRetryUseCase(repository port.RetryRepository, now func() time.Time, newID func() (string, error)) *ScheduleRetryUseCase {
	return &ScheduleRetryUseCase{repository: repository, now: now, newID: newID}
}

// Execute programa los intentos 2, 3 y 4 con demoras de 1, 2 y 4 segundos.
func (u *ScheduleRetryUseCase) Execute(ctx context.Context, consumerName string, envelope event.Envelope, attempt int, cause error) (port.ScheduleRetryResult, error) {
	if attempt < 1 || attempt >= 4 {
		return port.ScheduleRetryResult{}, errors.New("intento no programable")
	}
	messageID, err := u.newID()
	if err != nil {
		return port.ScheduleRetryResult{}, err
	}
	now := u.now().UTC()
	return u.repository.ScheduleRetry(ctx, port.ScheduleRetry{
		ConsumerName:  consumerName,
		EventID:       envelope.EventID,
		OrderID:       envelope.OrderID,
		CorrelationID: envelope.CorrelationID,
		Attempt:       attempt,
		MessageID:     messageID,
		NextAttempt:   now.Add(retryDelay(attempt)),
		Failure:       sanitizeFailure(cause),
		ScheduledAt:   now,
	})
}

func retryDelay(attempt int) time.Duration {
	return time.Second << (attempt - 1)
}

func sanitizeFailure(_ error) string {
	// El texto de un error arbitrario puede incluir SQL, credenciales u otros
	// datos internos. El consumidor solo programa reintentos para el error
	// tipado ErrTransientProcessing, así que persistimos su descripción fija.
	return port.ErrTransientProcessing.Error()
}
