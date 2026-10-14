package container

import (
	"context"
	"time"

	"eventflow/internal/order/application/usecase"
	"eventflow/internal/order/infrastructure/messaging"
	postgresorder "eventflow/internal/order/infrastructure/postgres"
	"eventflow/internal/order/presentation/handler"
	"eventflow/pkg/database"
	"eventflow/pkg/metrics"
	"eventflow/pkg/pubsub"
)

// OperationalMetrics lee los contadores durables requeridos por operación.
type OperationalMetrics struct {
	repository *postgresorder.OperationalMetrics
}

func NewOperationalMetrics(provider database.Database) (*OperationalMetrics, error) {
	repository, err := postgresorder.NewOperationalMetrics(provider)
	if err != nil {
		return nil, err
	}
	return &OperationalMetrics{repository: repository}, nil
}

func (m *OperationalMetrics) Read(ctx context.Context) (metrics.DurableStats, error) {
	return m.repository.Read(ctx)
}

// NewCreateOrderUseCase ensambla el caso de uso con el repositorio PostgreSQL.
func NewCreateOrderUseCase(provider database.Database) (*usecase.CreateOrderUseCase, error) {
	repository, err := postgresorder.NewOrderRepository(provider)
	if err != nil {
		return nil, err
	}
	return usecase.NewCreateOrderUseCase(repository, time.Now, usecase.NewUUID), nil
}

// NewOutboxRelay ensambla el relay del worker sobre PostgreSQL y RabbitMQ.
func NewOutboxRelay(provider database.Database, publisher pubsub.Publisher) (*messaging.Relay, error) {
	repository, err := postgresorder.NewOutboxRepository(provider)
	if err != nil {
		return nil, err
	}
	return messaging.NewRelay(repository, publisher, time.Now), nil
}

// NewOrderConsumer ensambla consumo manual y programación durable de reintentos.
func NewOrderConsumer(provider database.Database) (*messaging.Consumer, error) {
	repository, err := postgresorder.NewOrderRepository(provider)
	if err != nil {
		return nil, err
	}
	return messaging.NewConsumer(
		"orders-worker",
		usecase.NewProcessOrderUseCase(repository, time.Now),
		usecase.NewScheduleRetryUseCase(repository, time.Now, usecase.NewUUID),
		usecase.NewRecordTerminalFailureUseCase(repository, time.Now),
	), nil
}

// NewOrderHandler ensambla los casos de uso expuestos por HTTP.
func NewOrderHandler(provider database.Database) (*handler.OrderHandler, error) {
	repository, err := postgresorder.NewOrderRepository(provider)
	if err != nil {
		return nil, err
	}
	return handler.NewOrderHandler(
		usecase.NewCreateIdempotentOrderUseCase(repository, time.Now, usecase.NewUUID),
		usecase.NewGetOrderUseCase(repository),
	), nil
}
