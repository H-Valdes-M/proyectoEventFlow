package integration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"eventflow/cmd/config"
	"eventflow/cmd/container"
	"eventflow/internal/order/application/event"
	"eventflow/internal/order/infrastructure/messaging"
	postgresorder "eventflow/internal/order/infrastructure/postgres"
	"eventflow/pkg/database"
	"eventflow/pkg/pubsub"
	"github.com/rabbitmq/amqp091-go"
)

func TestRabbitMQProviderYRelayIntegracion(t *testing.T) {
	if os.Getenv("EVENTFLOW_MESSAGING_INTEGRATION") != "1" {
		t.Skip("definir EVENTFLOW_MESSAGING_INTEGRATION=1 para ejecutar contra PostgreSQL y RabbitMQ reales")
	}
	t.Chdir(filepath.Join("..", ".."))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	apiConfig, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal("cargar configuracion PostgreSQL")
	}
	workerConfig, err := config.LoadWorkerConfig()
	if err != nil {
		t.Fatal("cargar configuracion RabbitMQ")
	}
	db, err := database.NewPostgresDatabase(ctx, database.PostgresConfig{
		Host: apiConfig.DBHost, Port: apiConfig.DBPort, User: apiConfig.DBUser, Password: apiConfig.DBPassword, Database: apiConfig.DBName, SSLMode: apiConfig.DBSSLMode,
	})
	if err != nil {
		t.Fatal("crear PostgreSQL")
	}
	defer db.Close()
	pool, err := database.AsPostgresPool(db)
	if err != nil {
		t.Fatal("obtener pool PostgreSQL")
	}
	repository, err := postgresorder.NewOutboxRepository(db)
	if err != nil {
		t.Fatal("crear repositorio outbox")
	}
	publisher, err := pubsub.NewRabbitMQProvider(ctx, workerConfig.AMQPURL)
	if err != nil {
		t.Fatal("crear provider RabbitMQ")
	}
	defer publisher.Close()

	const messageID = "11111111-1111-4111-8111-111111111111"
	const eventID = "22222222-2222-4222-8222-222222222222"
	const orderID = "44444444-4444-4444-8444-444444444444"
	defer pool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID)
	defer pool.Exec(context.Background(), `DELETE FROM outbox_events WHERE message_id = $1`, messageID)
	if _, err := pool.Exec(ctx, `
INSERT INTO orders (id, customer_id, total_minor, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)`, orderID, "cliente-relay", 1, "pending", time.Now()); err != nil {
		t.Fatalf("insertar pedido de la prueba: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO outbox_events (message_id, event_id, event_type, order_id, payload, available_at, created_at)
VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)`, messageID, eventID, pubsub.RoutingKey, orderID, `{"event_id":"22222222-2222-4222-8222-222222222222","type":"order.created"}`, time.Now().Add(-time.Second), time.Now()); err != nil {
		t.Fatalf("insertar evento pendiente: %v", err)
	}
	relay := messaging.NewRelay(repository, publisher, time.Now)
	if err := relay.PublishPendingBatch(ctx, workerConfig.OutboxBatchSize); err != nil {
		t.Fatalf("publicar lote confirmado: %v", err)
	}
	var publishedAt *time.Time
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT published_at, publish_attempts FROM outbox_events WHERE message_id = $1`, messageID).Scan(&publishedAt, &attempts); err != nil {
		t.Fatalf("consultar outbox publicado: %v", err)
	}
	if publishedAt == nil || attempts != 1 {
		t.Fatalf("el relay debe marcar solo tras confirmar: published_at=%v attempts=%d", publishedAt, attempts)
	}
	if err := publisher.PublishConfirmed(ctx, pubsub.Message{MessageID: "33333333-3333-4333-8333-333333333333", EventID: "44444444-4444-4444-8444-444444444444", Type: "sin.ruta", Attempt: 1, Payload: []byte(`{}`)}); !errors.Is(err, pubsub.ErrUnroutable) {
		t.Fatalf("un mensaje mandatory sin ruta no puede confirmarse como éxito: %v", err)
	}
}

func TestConsumerManualProcesaYAckIntegraConRabbitMQ(t *testing.T) {
	if os.Getenv("EVENTFLOW_MESSAGING_INTEGRATION") != "1" {
		t.Skip("definir EVENTFLOW_MESSAGING_INTEGRATION=1 para ejecutar contra PostgreSQL y RabbitMQ reales")
	}
	t.Chdir(filepath.Join("..", ".."))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	apiConfig, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal("cargar configuracion PostgreSQL")
	}
	workerConfig, err := config.LoadWorkerConfig()
	if err != nil {
		t.Fatal("cargar configuracion RabbitMQ")
	}
	db, err := database.NewPostgresDatabase(ctx, database.PostgresConfig{Host: apiConfig.DBHost, Port: apiConfig.DBPort, User: apiConfig.DBUser, Password: apiConfig.DBPassword, Database: apiConfig.DBName, SSLMode: apiConfig.DBSSLMode})
	if err != nil {
		t.Fatal("crear PostgreSQL")
	}
	defer db.Close()
	pool, err := database.AsPostgresPool(db)
	if err != nil {
		t.Fatal("obtener pool PostgreSQL")
	}
	const orderID = "12121212-1212-4212-8212-121212121212"
	const eventID = "13131313-1313-4313-8313-131313131313"
	const messageID = "14141414-1414-4414-8414-141414141414"
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM notifications WHERE order_id = $1`, orderID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM event_processing WHERE event_id = $1`, eventID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID)
	}()
	if _, err := pool.Exec(ctx, `INSERT INTO orders (id, customer_id, total_minor, status, created_at, updated_at) VALUES ($1, 'cliente-consumo', 1, 'pending', $2, $2)`, orderID, time.Now().UTC()); err != nil {
		t.Fatal("preparar pedido para consumo")
	}
	provider, err := pubsub.NewRabbitMQProvider(ctx, workerConfig.AMQPURL)
	if err != nil {
		t.Fatal("crear provider RabbitMQ")
	}
	defer provider.Close()
	raw, err := amqp091.Dial(workerConfig.AMQPURL)
	if err != nil {
		t.Fatal("conectar para purgar cola de prueba")
	}
	rawChannel, err := raw.Channel()
	if err != nil {
		t.Fatal("abrir canal para purgar cola de prueba")
	}
	if _, err := rawChannel.QueuePurge(pubsub.Queue, false); err != nil {
		t.Fatal("purgar cola de prueba")
	}
	_ = rawChannel.Close()
	_ = raw.Close()
	consumer, err := container.NewOrderConsumer(db)
	if err != nil {
		t.Fatal("crear consumidor")
	}
	consumeCtx, stopConsume := context.WithCancel(ctx)
	finished := make(chan error, 1)
	go func() { finished <- provider.Consume(consumeCtx, consumeCtx, 1, consumer.HandleDelivery) }()
	envelope := event.NewOrderCreated(eventID, orderID, "correlation-consumer", time.Now())
	payload, err := envelope.Serialize()
	if err != nil {
		t.Fatal("serializar evento")
	}
	if err := provider.PublishConfirmed(ctx, pubsub.Message{MessageID: messageID, EventID: eventID, Type: event.OrderCreatedType, Attempt: 1, Payload: payload}); err != nil {
		t.Fatal("publicar evento para consumo")
	}
	for {
		var status string
		err = pool.QueryRow(ctx, `SELECT status FROM event_processing WHERE consumer_name = 'orders-worker' AND event_id = $1`, eventID).Scan(&status)
		if err == nil && status == "completed" {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("esperar procesamiento durable: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	stopConsume()
	if err := <-finished; err != nil {
		t.Fatalf("cerrar consumidor: %v", err)
	}
	var notifications int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE order_id = $1`, orderID).Scan(&notifications); err != nil || notifications != 1 {
		t.Fatalf("el efecto durable no quedo confirmado: count=%d err=%v", notifications, err)
	}
}

func TestConsumerManualRechazaEnvelopeInvalidoTrasRegistroDurable(t *testing.T) {
	if os.Getenv("EVENTFLOW_MESSAGING_INTEGRATION") != "1" {
		t.Skip("definir EVENTFLOW_MESSAGING_INTEGRATION=1 para ejecutar contra PostgreSQL y RabbitMQ reales")
	}
	t.Chdir(filepath.Join("..", ".."))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	apiConfig, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal("cargar configuracion PostgreSQL")
	}
	workerConfig, err := config.LoadWorkerConfig()
	if err != nil {
		t.Fatal("cargar configuracion RabbitMQ")
	}
	db, err := database.NewPostgresDatabase(ctx, database.PostgresConfig{Host: apiConfig.DBHost, Port: apiConfig.DBPort, User: apiConfig.DBUser, Password: apiConfig.DBPassword, Database: apiConfig.DBName, SSLMode: apiConfig.DBSSLMode})
	if err != nil {
		t.Fatal("crear PostgreSQL")
	}
	defer db.Close()
	pool, err := database.AsPostgresPool(db)
	if err != nil {
		t.Fatal("obtener pool PostgreSQL")
	}
	defer pool.Exec(context.Background(), `DELETE FROM rejected_deliveries WHERE consumer_name = 'orders-worker'`)
	provider, err := pubsub.NewRabbitMQProvider(ctx, workerConfig.AMQPURL)
	if err != nil {
		t.Fatal("crear provider")
	}
	defer provider.Close()
	raw, err := amqp091.Dial(workerConfig.AMQPURL)
	if err != nil {
		t.Fatal("conectar para publicar envelope invalido")
	}
	defer raw.Close()
	rawChannel, err := raw.Channel()
	if err != nil {
		t.Fatal("abrir canal crudo")
	}
	defer rawChannel.Close()
	if _, err := rawChannel.QueuePurge(pubsub.Queue, false); err != nil {
		t.Fatal("purgar cola de prueba")
	}
	consumer, err := container.NewOrderConsumer(db)
	if err != nil {
		t.Fatal("crear consumidor")
	}
	consumeCtx, stopConsume := context.WithCancel(ctx)
	finished := make(chan error, 1)
	go func() { finished <- provider.Consume(consumeCtx, consumeCtx, 1, consumer.HandleDelivery) }()
	if err := rawChannel.PublishWithContext(ctx, pubsub.Exchange, event.OrderCreatedType, false, false, amqp091.Publishing{ContentType: "application/json", Type: event.OrderCreatedType, Body: []byte(`{payload-secreto}`)}); err != nil {
		t.Fatal("publicar envelope invalido: ", err)
	}
	for {
		var count int
		err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM rejected_deliveries WHERE consumer_name = 'orders-worker' AND reason_sanitized = 'envelope invalido'`).Scan(&count)
		if err == nil && count == 1 {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("esperar rechazo durable: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	stopConsume()
	if err := <-finished; err != nil {
		t.Fatalf("cerrar consumidor: %v", err)
	}
}
