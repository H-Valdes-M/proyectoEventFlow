package messaging

import (
	"context"
	"errors"

	"eventflow/internal/order/application/event"
	"eventflow/internal/order/application/port"
	"eventflow/internal/order/application/usecase"
	"eventflow/pkg/pubsub"
	"go.uber.org/zap"
)

// Consumer adapta una entrega AMQP a los resultados durables de pedidos.
type Consumer struct {
	consumerName string
	process      *usecase.ProcessOrderUseCase
	retry        *usecase.ScheduleRetryUseCase
	terminal     *usecase.RecordTerminalFailureUseCase
	log          *zap.Logger
}

var testBeforeProcess func(pubsub.Delivery) error

// NewConsumer crea el único dueño de la disposición de las entregas.
func NewConsumer(consumerName string, process *usecase.ProcessOrderUseCase, retry *usecase.ScheduleRetryUseCase, terminal *usecase.RecordTerminalFailureUseCase) *Consumer {
	return &Consumer{consumerName: consumerName, process: process, retry: retry, terminal: terminal, log: zap.NewNop()}
}

// WithLogger añade correlación segura a los resultados del consumidor.
func (c *Consumer) WithLogger(log *zap.Logger) *Consumer {
	if log != nil {
		c.log = log
	}
	return c
}

// HandleDelivery confirma solo un completed durable y descarta una entrega
// reemplazada únicamente tras haber guardado su continuación en PostgreSQL.
func (c *Consumer) HandleDelivery(ctx context.Context, delivery pubsub.Delivery) (pubsub.Disposition, error) {
	envelope, err := event.Deserialize(delivery.Payload)
	if err != nil || envelope.EventID != delivery.EventID || envelope.Type != event.OrderCreatedType || delivery.Attempt < 1 {
		if err := c.terminal.Reject(ctx, c.consumerName, delivery.Payload); err != nil {
			return pubsub.KeepUnconfirmed, err
		}
		return pubsub.NackDrop, nil
	}
	c.log.Info("procesar entrega", zap.String("correlation_id", envelope.CorrelationID), zap.String("order_id", envelope.OrderID), zap.String("event_id", envelope.EventID), zap.Int("attempt", delivery.Attempt))
	if testBeforeProcess != nil {
		if err := testBeforeProcess(delivery); err != nil {
			if !errors.Is(err, port.ErrTransientProcessing) {
				return pubsub.KeepUnconfirmed, err
			}
			return c.handleTransient(ctx, envelope, delivery, err)
		}
	}
	result, err := c.process.ExecuteAttempt(ctx, c.consumerName, envelope, delivery.Attempt)
	if err == nil {
		if result.Duplicate {
			return pubsub.Ack, nil
		}
		if result.RetryScheduled {
			return pubsub.NackDrop, nil
		}
		if result.Terminal {
			return pubsub.NackDrop, nil
		}
		return pubsub.Ack, nil
	}
	if !errors.Is(err, port.ErrTransientProcessing) {
		return pubsub.KeepUnconfirmed, err
	}
	return c.handleTransient(ctx, envelope, delivery, err)
}

func (c *Consumer) handleTransient(ctx context.Context, envelope event.Envelope, delivery pubsub.Delivery, processingErr error) (pubsub.Disposition, error) {
	if delivery.Attempt == 4 {
		if err := c.terminal.Execute(ctx, c.consumerName, envelope, delivery.Attempt, processingErr); err != nil {
			return pubsub.KeepUnconfirmed, err
		}
		return pubsub.NackDrop, nil
	}
	retryResult, retryErr := c.retry.Execute(ctx, c.consumerName, envelope, delivery.Attempt, processingErr)
	if retryErr != nil {
		return pubsub.KeepUnconfirmed, retryErr
	}
	if retryResult.Completed {
		return pubsub.Ack, nil
	}
	return pubsub.NackDrop, nil
}
