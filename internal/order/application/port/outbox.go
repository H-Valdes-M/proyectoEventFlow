package port

import (
	"context"
	"time"
)

// OutboxEvent es el evento durable pendiente de publicación.
type OutboxEvent struct {
	MessageID string
	EventID   string
	Type      string
	Attempt   int
	Payload   []byte
}

// OutboxRepository conserva el estado durable que controla el relay.
type OutboxRepository interface {
	ReadReady(context.Context, time.Time, int) ([]OutboxEvent, error)
	MarkPublished(context.Context, string, time.Time) error
}
