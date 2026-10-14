package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"eventflow/internal/order/application/event"
	"eventflow/internal/order/application/port"
)

// RecordTerminalFailureUseCase conserva el agotamiento antes de retirar la entrega.
type RecordTerminalFailureUseCase struct {
	repository port.TerminalRepository
	now        func() time.Time
}

const terminalFailureReason = "fallo de negocio agotado"

// NewRecordTerminalFailureUseCase crea el caso de uso con reloj inyectable.
func NewRecordTerminalFailureUseCase(repository port.TerminalRepository, now func() time.Time) *RecordTerminalFailureUseCase {
	return &RecordTerminalFailureUseCase{repository: repository, now: now}
}

// Execute registra el cuarto fallo de negocio de un evento validado.
func (u *RecordTerminalFailureUseCase) Execute(ctx context.Context, consumerName string, envelope event.Envelope, attempt int, _ error) error {
	if attempt != 4 {
		return errors.New("intento terminal invalido")
	}
	return u.repository.RecordTerminalFailure(ctx, port.TerminalFailure{
		ConsumerName: consumerName,
		EventID:      envelope.EventID,
		OrderID:      envelope.OrderID,
		Attempt:      attempt,
		Failure:      terminalFailureReason,
		FailedAt:     u.now().UTC(),
	})
}

// Reject registra solo una huella del cuerpo inválido, nunca su contenido.
func (u *RecordTerminalFailureUseCase) Reject(ctx context.Context, consumerName string, payload []byte) error {
	digest := sha256.Sum256(payload)
	return u.repository.RecordRejectedDelivery(ctx, port.RejectedDelivery{
		ConsumerName:  consumerName,
		PayloadDigest: hex.EncodeToString(digest[:]),
		Reason:        "envelope invalido",
		RejectedAt:    u.now().UTC(),
	})
}
