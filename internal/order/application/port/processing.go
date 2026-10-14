package port

import (
	"context"
	"errors"
	"time"
)

var ErrTransientProcessing = errors.New("fallo transitorio de procesamiento")

// ProcessOrder representa el efecto durable solicitado por una entrega de evento.
type ProcessOrder struct {
	ConsumerName  string
	EventID       string
	OrderID       string
	CorrelationID string
	Attempt       int
	ProcessedAt   time.Time
}

// ProcessOrderResult informa si esta ejecución confirmó el efecto o era duplicada.
type ProcessOrderResult struct {
	Duplicate      bool
	RetryScheduled bool
	Terminal       bool
}

// ProcessingRepository conserva el efecto y la deduplicación en una sola transacción.
type ProcessingRepository interface {
	Process(context.Context, ProcessOrder) (ProcessOrderResult, error)
}

// ScheduleRetry describe la persistencia de un nuevo intento de transporte.
type ScheduleRetry struct {
	ConsumerName  string
	EventID       string
	OrderID       string
	CorrelationID string
	Attempt       int
	MessageID     string
	NextAttempt   time.Time
	Failure       string
	ScheduledAt   time.Time
}

// ScheduleRetryResult permite retirar con seguridad una entrega sustituida.
type ScheduleRetryResult struct {
	Completed        bool
	AlreadyScheduled bool
}

// RetryRepository programa el siguiente intento junto con su outbox.
type RetryRepository interface {
	ScheduleRetry(context.Context, ScheduleRetry) (ScheduleRetryResult, error)
}

// TerminalFailure describe el agotamiento durable de un evento de pedido.
type TerminalFailure struct {
	ConsumerName string
	EventID      string
	OrderID      string
	Attempt      int
	Failure      string
	FailedAt     time.Time
}

// RejectedDelivery conserva evidencia mínima de una entrega que no se pudo validar.
type RejectedDelivery struct {
	ConsumerName  string
	PayloadDigest string
	Reason        string
	RejectedAt    time.Time
}

// TerminalRepository registra resultados terminales y rechazos sin payload.
type TerminalRepository interface {
	RecordTerminalFailure(context.Context, TerminalFailure) error
	RecordRejectedDelivery(context.Context, RejectedDelivery) error
}
