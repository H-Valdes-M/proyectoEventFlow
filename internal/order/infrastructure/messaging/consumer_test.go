package messaging

import (
	"context"
	"errors"
	"testing"
	"time"

	"eventflow/internal/order/application/event"
	"eventflow/internal/order/application/port"
	"eventflow/internal/order/application/usecase"
	"eventflow/pkg/pubsub"
)

func TestConsumerConfirmaCompletadoODuplicadoYConservaFalloDePersistencia(t *testing.T) {
	envelope := event.NewOrderCreated("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "correlation", time.Now())
	payload, _ := envelope.Serialize()
	delivery := pubsub.Delivery{EventID: envelope.EventID, Type: envelope.Type, Attempt: 1, Payload: payload}
	for name, testCase := range map[string]struct {
		result     port.ProcessOrderResult
		processErr error
		want       pubsub.Disposition
	}{
		"completado":   {want: pubsub.Ack},
		"duplicado":    {result: port.ProcessOrderResult{Duplicate: true}, want: pubsub.Ack},
		"persistencia": {processErr: errors.New("base no disponible"), want: pubsub.KeepUnconfirmed},
	} {
		t.Run(name, func(t *testing.T) {
			consumer := testConsumer(&consumerProcessingDouble{result: testCase.result, err: testCase.processErr}, &consumerRetryDouble{}, &consumerTerminalDouble{})
			got, _ := consumer.HandleDelivery(context.Background(), delivery)
			if got != testCase.want {
				t.Fatalf("disposicion inesperada: got=%v want=%v", got, testCase.want)
			}
		})
	}
}

func TestConsumerRechazaSoloTrasProgramarReintentoDurable(t *testing.T) {
	envelope := event.NewOrderCreated("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "correlation", time.Now())
	payload, _ := envelope.Serialize()
	retry := &consumerRetryDouble{}
	consumer := testConsumer(&consumerProcessingDouble{err: port.ErrTransientProcessing}, retry, &consumerTerminalDouble{})
	got, err := consumer.HandleDelivery(context.Background(), pubsub.Delivery{EventID: envelope.EventID, Type: envelope.Type, Attempt: 1, Payload: payload})
	if err != nil || got != pubsub.NackDrop || retry.calls != 1 {
		t.Fatalf("reintento durable inesperado: disposition=%v err=%v calls=%d", got, err, retry.calls)
	}

	retry.err = errors.New("base no disponible")
	got, err = consumer.HandleDelivery(context.Background(), pubsub.Delivery{EventID: envelope.EventID, Type: envelope.Type, Attempt: 1, Payload: payload})
	if err == nil || got != pubsub.KeepUnconfirmed {
		t.Fatalf("sin commit no debe descartarse: disposition=%v err=%v", got, err)
	}
}

func TestConsumerRegistraFalloTerminalYRechazoAntesDeDescartar(t *testing.T) {
	envelope := event.NewOrderCreated("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "correlation", time.Now())
	payload, _ := envelope.Serialize()
	terminal := &consumerTerminalDouble{}
	consumer := testConsumer(&consumerProcessingDouble{err: port.ErrTransientProcessing}, &consumerRetryDouble{}, terminal)

	got, err := consumer.HandleDelivery(context.Background(), pubsub.Delivery{EventID: envelope.EventID, Type: envelope.Type, Attempt: 4, Payload: payload})
	if err != nil || got != pubsub.NackDrop || terminal.terminalCalls != 1 {
		t.Fatalf("fallo terminal inesperado: disposition=%v err=%v calls=%d", got, err, terminal.terminalCalls)
	}
	got, err = consumer.HandleDelivery(context.Background(), pubsub.Delivery{Payload: []byte(`{secreto}`)})
	if err != nil || got != pubsub.NackDrop || terminal.rejectedCalls != 1 {
		t.Fatalf("rechazo invalido inesperado: disposition=%v err=%v calls=%d", got, err, terminal.rejectedCalls)
	}
	terminal.err = errors.New("base no disponible")
	got, err = consumer.HandleDelivery(context.Background(), pubsub.Delivery{Payload: []byte(`{secreto}`)})
	if err == nil || got != pubsub.KeepUnconfirmed {
		t.Fatalf("sin persistencia el rechazo no debe descartarse: disposition=%v err=%v", got, err)
	}
}

func TestConsumerDescartaDuplicadoTerminalSinReiniciarPresupuesto(t *testing.T) {
	envelope := event.NewOrderCreated("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "correlation", time.Now())
	payload, _ := envelope.Serialize()
	terminal := &consumerTerminalDouble{}
	consumer := testConsumer(&consumerProcessingDouble{result: port.ProcessOrderResult{Terminal: true}}, &consumerRetryDouble{}, terminal)
	got, err := consumer.HandleDelivery(context.Background(), pubsub.Delivery{EventID: envelope.EventID, Type: envelope.Type, Attempt: 4, Payload: payload})
	if err != nil || got != pubsub.NackDrop || terminal.terminalCalls != 0 {
		t.Fatalf("duplicado terminal inesperado: disposition=%v err=%v calls=%d", got, err, terminal.terminalCalls)
	}
}

func testConsumer(process port.ProcessingRepository, retry port.RetryRepository, terminal port.TerminalRepository) *Consumer {
	return NewConsumer("orders-worker", usecase.NewProcessOrderUseCase(process, time.Now), usecase.NewScheduleRetryUseCase(retry, time.Now, func() (string, error) { return "33333333-3333-4333-8333-333333333333", nil }), usecase.NewRecordTerminalFailureUseCase(terminal, time.Now))
}

type consumerProcessingDouble struct {
	result port.ProcessOrderResult
	err    error
}

func (d *consumerProcessingDouble) Process(context.Context, port.ProcessOrder) (port.ProcessOrderResult, error) {
	return d.result, d.err
}

type consumerRetryDouble struct {
	calls int
	err   error
}

func (d *consumerRetryDouble) ScheduleRetry(context.Context, port.ScheduleRetry) (port.ScheduleRetryResult, error) {
	d.calls++
	return port.ScheduleRetryResult{}, d.err
}

type consumerTerminalDouble struct {
	terminalCalls int
	rejectedCalls int
	err           error
}

func (d *consumerTerminalDouble) RecordTerminalFailure(context.Context, port.TerminalFailure) error {
	d.terminalCalls++
	return d.err
}

func (d *consumerTerminalDouble) RecordRejectedDelivery(context.Context, port.RejectedDelivery) error {
	d.rejectedCalls++
	return d.err
}
