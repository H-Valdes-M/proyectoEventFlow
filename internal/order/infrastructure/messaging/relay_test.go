package messaging

import (
	"context"
	"errors"
	"testing"
	"time"

	"eventflow/internal/order/application/port"
	"eventflow/pkg/pubsub"
)

func TestRelayNoMarcaEventoSiLaPublicacionFallaYConservaEventID(t *testing.T) {
	now := time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC)
	event := port.OutboxEvent{MessageID: "mensaje-1", EventID: "evento-1", Type: pubsub.RoutingKey, Payload: []byte(`{"event_id":"evento-1"}`)}
	repository := &outboxDouble{events: []port.OutboxEvent{event}}
	publisher := &publisherDouble{err: errors.New("broker no disponible")}
	relay := NewRelay(repository, publisher, func() time.Time { return now })

	if err := relay.PublishPendingBatch(context.Background(), 20); err == nil {
		t.Fatal("un fallo de publicación debe llegar al worker")
	}
	if len(repository.marked) != 0 {
		t.Fatalf("un evento no confirmado no debe marcarse publicado: %+v", repository.marked)
	}
	if len(publisher.messages) != 1 || publisher.messages[0].EventID != event.EventID {
		t.Fatalf("el event_id debe conservarse para el reintento: %+v", publisher.messages)
	}

	publisher.err = nil
	if err := relay.PublishPendingBatch(context.Background(), 20); err != nil {
		t.Fatalf("republicar el evento recuperable: %v", err)
	}
	if len(repository.marked) != 1 || repository.marked[0] != event.MessageID {
		t.Fatalf("solo el evento confirmado debe marcarse: %+v", repository.marked)
	}
	if publisher.messages[1].EventID != event.EventID {
		t.Fatalf("la republicación cambió el event_id: %+v", publisher.messages)
	}
}

type outboxDouble struct {
	events []port.OutboxEvent
	marked []string
}

func (d *outboxDouble) ReadReady(context.Context, time.Time, int) ([]port.OutboxEvent, error) {
	return d.events, nil
}

func (d *outboxDouble) MarkPublished(_ context.Context, messageID string, _ time.Time) error {
	d.marked = append(d.marked, messageID)
	return nil
}

type publisherDouble struct {
	messages []pubsub.Message
	err      error
}

func (d *publisherDouble) PublishConfirmed(_ context.Context, message pubsub.Message) error {
	d.messages = append(d.messages, message)
	return d.err
}
