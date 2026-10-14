package event

import (
	"encoding/json"
	"time"
)

const OrderCreatedType = "order.created"

// Envelope representa un evento lógico independiente de sus mensajes de transporte.
type Envelope struct {
	EventID       string    `json:"event_id"`
	Type          string    `json:"type"`
	Version       int       `json:"version"`
	OccurredAt    time.Time `json:"occurred_at"`
	OrderID       string    `json:"order_id"`
	CorrelationID string    `json:"correlation_id"`
}

// NewOrderCreated construye el evento inicial de un pedido pendiente.
func NewOrderCreated(eventID, orderID, correlationID string, occurredAt time.Time) Envelope {
	return Envelope{
		EventID:       eventID,
		Type:          OrderCreatedType,
		Version:       1,
		OccurredAt:    occurredAt.UTC(),
		OrderID:       orderID,
		CorrelationID: correlationID,
	}
}

// Serialize codifica el envelope para almacenarlo en el outbox.
func (e Envelope) Serialize() ([]byte, error) {
	return json.Marshal(e)
}

// Deserialize decodifica un envelope almacenado en el outbox.
func Deserialize(payload []byte) (Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}
