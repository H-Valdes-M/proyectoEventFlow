package postgres_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"eventflow/cmd/config"
	"eventflow/internal/order/application/dto"
	"eventflow/internal/order/application/event"
	"eventflow/internal/order/application/port"
	"eventflow/internal/order/application/usecase"
	"eventflow/internal/order/domain/entity"
	"eventflow/internal/order/domain/repository"
	postgresorder "eventflow/internal/order/infrastructure/postgres"
	"eventflow/pkg/apperrors"
	"eventflow/pkg/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOrderRepositoryIntegracion(t *testing.T) {
	if os.Getenv("EVENTFLOW_PG_INTEGRATION") != "1" {
		t.Skip("definir EVENTFLOW_PG_INTEGRATION=1 para ejecutar contra PostgreSQL real")
	}
	t.Chdir(filepath.Join("..", "..", "..", ".."))
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal("cargar configuracion de integración")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	db, err := database.NewPostgresDatabase(ctx, database.PostgresConfig{
		Host: cfg.DBHost, Port: cfg.DBPort, User: cfg.DBUser, Password: cfg.DBPassword, Database: cfg.DBName, SSLMode: cfg.DBSSLMode,
	})
	if err != nil {
		t.Fatal("crear provider PostgreSQL")
	}
	defer db.Close()
	repo, err := postgresorder.NewOrderRepository(db)
	if err != nil {
		t.Fatal("crear repositorio")
	}
	pool, err := database.AsPostgresPool(db)
	if err != nil {
		t.Fatal("obtener pool PostgreSQL")
	}
	validID := "44444444-4444-4444-8444-444444444444"
	failedID := "55555555-5555-4555-8555-555555555555"
	defer pool.Exec(context.Background(), `DELETE FROM outbox_events WHERE order_id IN ($1, $2)`, validID, failedID)
	defer pool.Exec(context.Background(), `DELETE FROM orders WHERE id IN ($1, $2)`, validID, failedID)

	now := time.Date(2026, 10, 13, 18, 0, 0, 0, time.UTC)
	valid := entity.Order{
		ID: validID, CustomerID: "cliente-repositorio", TotalMinor: 3600, Status: entity.OrderPending, CreatedAt: now, UpdatedAt: now,
		Items: []entity.OrderItem{{ProductID: "producto-b", Quantity: 2, UnitPriceMinor: 1500}, {ProductID: "producto-a", Quantity: 3, UnitPriceMinor: 200}},
	}
	if err := repo.Save(ctx, valid); err != nil {
		t.Fatalf("guardar pedido válido: %v", err)
	}
	found, err := repo.FindByID(ctx, validID)
	if err != nil || found.ID != validID || len(found.Items) != 2 || found.Items[0].ProductID != "producto-a" || found.Items[1].ProductID != "producto-b" {
		t.Fatalf("pedido reconstruido inesperado: order=%+v err=%v", found, err)
	}
	if _, err := repo.FindByID(ctx, "66666666-6666-4666-8666-666666666666"); !errors.Is(err, repository.ErrOrderNotFound) {
		t.Fatalf("ausencia inesperada: %v", err)
	}
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := repo.FindByID(canceledCtx, validID); !errors.Is(err, apperrors.ErrInternal) {
		t.Fatalf("un fallo operativo no debe ser ausencia: %v", err)
	}
	failed := entity.Order{
		ID: failedID, CustomerID: "cliente-rollback", TotalMinor: 1, Status: entity.OrderPending, CreatedAt: now, UpdatedAt: now,
		Items: []entity.OrderItem{{ProductID: "articulo-invalido", Quantity: 1, UnitPriceMinor: 0}},
	}
	if err := repo.Save(ctx, failed); err == nil {
		t.Fatal("el articulo inválido debe revertir el pedido")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE id = $1`, failedID).Scan(&count); err != nil || count != 0 {
		t.Fatal("el rollback no debe dejar pedido parcial")
	}
	testIdempotenciaIntegracion(t, ctx, repo, pool)
}

func TestProcessOrderIntegracion(t *testing.T) {
	if os.Getenv("EVENTFLOW_PG_INTEGRATION") != "1" {
		t.Skip("definir EVENTFLOW_PG_INTEGRATION=1 para ejecutar contra PostgreSQL real")
	}
	t.Chdir(filepath.Join("..", "..", "..", ".."))
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal("cargar configuracion de integración")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := database.NewPostgresDatabase(ctx, database.PostgresConfig{
		Host: cfg.DBHost, Port: cfg.DBPort, User: cfg.DBUser, Password: cfg.DBPassword, Database: cfg.DBName, SSLMode: cfg.DBSSLMode,
	})
	if err != nil {
		t.Fatal("crear provider PostgreSQL")
	}
	defer db.Close()
	repo, err := postgresorder.NewOrderRepository(db)
	if err != nil {
		t.Fatal("crear repositorio")
	}
	pool, err := database.AsPostgresPool(db)
	if err != nil {
		t.Fatal("obtener pool PostgreSQL")
	}
	const processedOrderID = "10101010-1010-4010-8010-101010101010"
	const failedOrderID = "30303030-3030-4030-8030-303030303030"
	const concurrentOrderID = "50505050-5050-4050-8050-505050505050"
	const retryOrderID = "70707070-7070-4070-8070-707070707070"
	const eventID = "20202020-2020-4020-8020-202020202020"
	const failedEventID = "40404040-4040-4040-8040-404040404040"
	const concurrentEventID = "60606060-6060-4060-8060-606060606060"
	const retryEventID = "80808080-8080-4080-8080-808080808080"
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_events WHERE order_id IN ($1, $2, $3, $4)`, processedOrderID, failedOrderID, concurrentOrderID, retryOrderID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM notifications WHERE order_id IN ($1, $2, $3, $4)`, processedOrderID, failedOrderID, concurrentOrderID, retryOrderID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM event_processing WHERE order_id IN ($1, $2, $3, $4)`, processedOrderID, failedOrderID, concurrentOrderID, retryOrderID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM orders WHERE id IN ($1, $2, $3, $4)`, processedOrderID, failedOrderID, concurrentOrderID, retryOrderID)
	}()

	now := time.Date(2026, 10, 13, 21, 0, 0, 0, time.UTC)
	for _, order := range []entity.Order{
		{ID: processedOrderID, CustomerID: "cliente-procesado", TotalMinor: 100, Status: entity.OrderPending, CreatedAt: now, UpdatedAt: now, Items: []entity.OrderItem{{ProductID: "producto-procesado", Quantity: 1, UnitPriceMinor: 100}}},
		{ID: failedOrderID, CustomerID: "cliente-fallido", TotalMinor: 100, Status: "failed", CreatedAt: now, UpdatedAt: now, Items: []entity.OrderItem{{ProductID: "producto-fallido", Quantity: 1, UnitPriceMinor: 100}}},
	} {
		if err := repo.Save(ctx, order); err != nil {
			t.Fatalf("preparar pedido %s: %v", order.ID, err)
		}
	}
	process := usecase.NewProcessOrderUseCase(repo, func() time.Time { return now.Add(time.Minute) })
	envelope := event.NewOrderCreated(eventID, processedOrderID, processedOrderID, now)

	first, err := process.Execute(ctx, "orders-worker", envelope)
	if err != nil || first.Duplicate {
		t.Fatalf("primer procesamiento inesperado: result=%+v err=%v", first, err)
	}
	second, err := process.Execute(ctx, "orders-worker", envelope)
	if err != nil || !second.Duplicate {
		t.Fatalf("duplicado inesperado: result=%+v err=%v", second, err)
	}
	var notifications int
	var status string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE order_id = $1`, processedOrderID).Scan(&notifications); err != nil || notifications != 1 {
		t.Fatalf("notificaciones inesperadas: count=%d err=%v", notifications, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status, attempts FROM event_processing WHERE consumer_name = 'orders-worker' AND event_id = $1`, eventID).Scan(&status, &attempts); err != nil || status != "completed" || attempts != 1 {
		t.Fatalf("procesamiento inesperado: status=%s attempts=%d err=%v", status, attempts, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, processedOrderID).Scan(&status); err != nil || status != "processed" {
		t.Fatalf("pedido inesperado: status=%s err=%v", status, err)
	}
	concurrentOrder := entity.Order{ID: concurrentOrderID, CustomerID: "cliente-concurrente", TotalMinor: 100, Status: entity.OrderPending, CreatedAt: now, UpdatedAt: now, Items: []entity.OrderItem{{ProductID: "producto-concurrente", Quantity: 1, UnitPriceMinor: 100}}}
	if err := repo.Save(ctx, concurrentOrder); err != nil {
		t.Fatalf("preparar pedido concurrente: %v", err)
	}
	start := make(chan struct{})
	results := make(chan port.ProcessOrderResult, 2)
	errorsCh := make(chan error, 2)
	concurrentEnvelope := event.NewOrderCreated(concurrentEventID, concurrentOrderID, concurrentOrderID, now)
	for range 2 {
		go func() {
			<-start
			result, err := process.Execute(ctx, "orders-worker", concurrentEnvelope)
			results <- result
			errorsCh <- err
		}()
	}
	close(start)
	completed := 0
	duplicates := 0
	for range 2 {
		result := <-results
		if err := <-errorsCh; err != nil {
			t.Fatalf("procesamiento concurrente: %v", err)
		}
		if result.Duplicate {
			duplicates++
		} else {
			completed++
		}
	}
	if completed != 1 || duplicates != 1 {
		t.Fatalf("resultado concurrente inesperado: completados=%d duplicados=%d", completed, duplicates)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE order_id = $1`, concurrentOrderID).Scan(&notifications); err != nil || notifications != 1 {
		t.Fatalf("la concurrencia duplico el efecto: count=%d err=%v", notifications, err)
	}

	failedEnvelope := event.NewOrderCreated(failedEventID, failedOrderID, failedOrderID, now)
	if _, err := process.Execute(ctx, "orders-worker", failedEnvelope); err == nil {
		t.Fatal("un pedido en estado terminal no debe confirmar el procesamiento")
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE order_id = $1`, failedOrderID).Scan(&notifications); err != nil || notifications != 0 {
		t.Fatalf("el rollback dejo notificacion: count=%d err=%v", notifications, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM event_processing WHERE event_id = $1`, failedEventID).Scan(&notifications); err != nil || notifications != 0 {
		t.Fatalf("el rollback dejo estado de evento: count=%d err=%v", notifications, err)
	}

	retryOrder := entity.Order{ID: retryOrderID, CustomerID: "cliente-reintento", TotalMinor: 100, Status: entity.OrderPending, CreatedAt: now, UpdatedAt: now, Items: []entity.OrderItem{{ProductID: "producto-reintento", Quantity: 1, UnitPriceMinor: 100}}}
	if err := repo.Save(ctx, retryOrder); err != nil {
		t.Fatalf("preparar pedido reintento: %v", err)
	}
	retryEnvelope := event.NewOrderCreated(retryEventID, retryOrderID, "correlation-retry", now)
	schedule := usecase.NewScheduleRetryUseCase(repo, func() time.Time { return now }, func() (string, error) { return "90909090-9090-4090-8090-909090909090", nil })
	if _, err := schedule.Execute(ctx, "orders-worker", retryEnvelope, 1, errors.New("fallo temporal")); err != nil {
		t.Fatalf("programar reintento durable: %v", err)
	}
	if _, err := schedule.Execute(ctx, "orders-worker", retryEnvelope, 1, errors.New("duplicado")); err != nil {
		t.Fatalf("duplicado de reintento: %v", err)
	}
	var nextAttempt time.Time
	if err := pool.QueryRow(ctx, `SELECT status, attempts, next_attempt_at FROM event_processing WHERE consumer_name = 'orders-worker' AND event_id = $1`, retryEventID).Scan(&status, &attempts, &nextAttempt); err != nil || status != "retry_scheduled" || attempts != 1 || !nextAttempt.Equal(now.Add(time.Second)) {
		t.Fatalf("programacion durable inesperada: status=%s attempts=%d next=%v err=%v", status, attempts, nextAttempt, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_id = $1 AND attempt = 2`, retryEventID).Scan(&notifications); err != nil || notifications != 1 {
		t.Fatalf("el duplicado creo otro outbox: count=%d err=%v", notifications, err)
	}
	earlyProcess := usecase.NewProcessOrderUseCase(repo, func() time.Time { return now })
	early, err := earlyProcess.ExecuteAttempt(ctx, "orders-worker", retryEnvelope, 2)
	if err != nil || !early.RetryScheduled {
		t.Fatalf("no debe ejecutarse antes de next_attempt_at: result=%+v err=%v", early, err)
	}
	afterDelay := usecase.NewProcessOrderUseCase(repo, func() time.Time { return now.Add(time.Second) })
	if result, err := afterDelay.ExecuteAttempt(ctx, "orders-worker", retryEnvelope, 2); err != nil || result.Duplicate || result.RetryScheduled {
		t.Fatalf("el reintento listo debe completar: result=%+v err=%v", result, err)
	}
}

func TestRecordTerminalFailureIntegracion(t *testing.T) {
	if os.Getenv("EVENTFLOW_PG_INTEGRATION") != "1" {
		t.Skip("definir EVENTFLOW_PG_INTEGRATION=1 para ejecutar contra PostgreSQL real")
	}
	t.Chdir(filepath.Join("..", "..", "..", ".."))
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal("cargar configuracion de integración")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := database.NewPostgresDatabase(ctx, database.PostgresConfig{Host: cfg.DBHost, Port: cfg.DBPort, User: cfg.DBUser, Password: cfg.DBPassword, Database: cfg.DBName, SSLMode: cfg.DBSSLMode})
	if err != nil {
		t.Fatal("crear provider PostgreSQL")
	}
	defer db.Close()
	repo, err := postgresorder.NewOrderRepository(db)
	if err != nil {
		t.Fatal("crear repositorio")
	}
	pool, err := database.AsPostgresPool(db)
	if err != nil {
		t.Fatal("obtener pool PostgreSQL")
	}
	const orderID = "91919191-9191-4191-8191-919191919191"
	const eventID = "92929292-9292-4292-8292-929292929292"
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM failed_events WHERE order_id = $1`, orderID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM event_processing WHERE order_id = $1`, orderID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM rejected_deliveries WHERE consumer_name = 'orders-worker'`)
	}()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `INSERT INTO orders (id, customer_id, total_minor, status, created_at, updated_at) VALUES ($1, 'cliente-terminal', 1, 'pending', $2, $2)`, orderID, now); err != nil {
		t.Fatal("preparar pedido terminal: ", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO event_processing (consumer_name, event_id, order_id, status, attempts, created_at, updated_at) VALUES ('orders-worker', $1, $2, 'ready', 4, $3, $3)`, eventID, orderID, now); err != nil {
		t.Fatal("preparar cuarto intento: ", err)
	}
	terminal := usecase.NewRecordTerminalFailureUseCase(repo, func() time.Time { return now })
	envelope := event.NewOrderCreated(eventID, orderID, "correlation-terminal", now)
	if err := terminal.Execute(ctx, "orders-worker", envelope, 4, errors.New("password=secreto")); err != nil {
		t.Fatal("registrar fallo terminal: ", err)
	}
	if err := terminal.Execute(ctx, "orders-worker", envelope, 4, errors.New("no debe reemplazar evidencia")); err != nil {
		t.Fatal("repetir fallo terminal: ", err)
	}
	var orderStatus, processingStatus, reason string
	if err := pool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&orderStatus); err != nil || orderStatus != "failed" {
		t.Fatalf("pedido terminal inesperado: status=%s err=%v", orderStatus, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status, last_error_sanitized FROM event_processing WHERE consumer_name = 'orders-worker' AND event_id = $1`, eventID).Scan(&processingStatus, &reason); err != nil || processingStatus != "failed" || reason != "fallo de negocio agotado" {
		t.Fatalf("procesamiento terminal inesperado: status=%s reason=%q err=%v", processingStatus, reason, err)
	}
	if err := pool.QueryRow(ctx, `SELECT reason_sanitized FROM failed_events WHERE consumer_name = 'orders-worker' AND event_id = $1`, eventID).Scan(&reason); err != nil || reason != "fallo de negocio agotado" {
		t.Fatalf("evidencia terminal inesperada: reason=%q err=%v", reason, err)
	}
	if err := terminal.Reject(ctx, "orders-worker", []byte("body-que-no-se-guarda")); err != nil {
		t.Fatal("registrar rechazo: ", err)
	}
	var digest, rejectedReason string
	if err := pool.QueryRow(ctx, `SELECT payload_digest, reason_sanitized FROM rejected_deliveries WHERE consumer_name = 'orders-worker'`).Scan(&digest, &rejectedReason); err != nil || len(digest) != 64 || rejectedReason != "envelope invalido" {
		t.Fatalf("rechazo persistido inesperado: digest=%q reason=%q err=%v", digest, rejectedReason, err)
	}
}

func testIdempotenciaIntegracion(t *testing.T, ctx context.Context, repo *postgresorder.OrderRepository, pool *pgxpool.Pool) {
	t.Helper()
	const key = "integracion-idempotencia-1"
	const rollbackKey = "integracion-idempotencia-rollback"
	const orderID = "99999999-9999-4999-8999-999999999999"
	const retryID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	defer pool.Exec(context.Background(), `DELETE FROM orders WHERE id IN ($1, $2)`, orderID, retryID)
	defer pool.Exec(context.Background(), `DELETE FROM outbox_events WHERE order_id IN ($1, $2)`, orderID, retryID)
	defer pool.Exec(context.Background(), `DELETE FROM idempotency_keys WHERE idempotency_key IN ($1, $2)`, key, rollbackKey)
	now := time.Date(2026, 10, 13, 19, 0, 0, 0, time.UTC)
	order := entity.Order{ID: orderID, CustomerID: "cliente-idempotente", TotalMinor: 3000, Status: entity.OrderPending, CreatedAt: now, UpdatedAt: now, Items: []entity.OrderItem{{ProductID: "producto", Quantity: 2, UnitPriceMinor: 1500}}}
	snapshot := dto.CreateOrderOutput{ID: orderID, Status: "pending", TotalMinor: 3000}
	createdInput := idempotentInput(key, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", order, snapshot, "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222")
	created, err := repo.CreateIdempotent(ctx, createdInput)
	if err != nil || created.Replay || created.Output != snapshot {
		t.Fatalf("creacion idempotente inesperada: result=%+v err=%v", created, err)
	}
	var persistedEventID string
	var payload []byte
	var publishedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT event_id, payload, published_at FROM outbox_events WHERE message_id = $1`, createdInput.MessageID).Scan(&persistedEventID, &payload, &publishedAt); err != nil {
		t.Fatal("consultar evento pendiente: ", err)
	}
	persistedEvent, err := event.Deserialize(payload)
	if err != nil || persistedEventID != createdInput.Event.EventID || persistedEvent != createdInput.Event || publishedAt != nil {
		t.Fatalf("evento pendiente inesperado: event=%+v id=%s published_at=%v err=%v", persistedEvent, persistedEventID, publishedAt, err)
	}
	replayOrder := entity.Order{ID: retryID}
	replay, err := repo.CreateIdempotent(ctx, idempotentInput(key, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", replayOrder, dto.CreateOrderOutput{ID: retryID}, "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444"))
	if err != nil || !replay.Replay || replay.Output != snapshot {
		t.Fatalf("replay inesperado: result=%+v err=%v", replay, err)
	}
	if _, err := repo.CreateIdempotent(ctx, idempotentInput(key, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", order, snapshot, "55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666")); !errors.Is(err, port.ErrIdempotencyConflict) {
		t.Fatalf("la misma clave con otra huella debe causar conflicto: %v", err)
	}
	broken := order
	broken.ID = retryID
	broken.CustomerID = "cliente-rollback-outbox"
	broken.Items = []entity.OrderItem{{ProductID: "valido", Quantity: 1, UnitPriceMinor: 1}}
	broken.TotalMinor = 1
	brokenSnapshot := dto.CreateOrderOutput{ID: retryID, Status: "pending", TotalMinor: 1}
	brokenInput := idempotentInput(rollbackKey, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", broken, brokenSnapshot, "77777777-7777-4777-8777-777777777777", "no-es-un-uuid")
	if _, err := repo.CreateIdempotent(ctx, brokenInput); err == nil {
		t.Fatal("un fallo al insertar outbox debe revertir la transaccion")
	}
	var orders, keys, outbox int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM orders WHERE id = $1`, retryID).Scan(&orders); err != nil || orders != 0 {
		t.Fatalf("rollback dejo pedido: count=%d err=%v", orders, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM idempotency_keys WHERE idempotency_key = $1`, rollbackKey).Scan(&keys); err != nil || keys != 0 {
		t.Fatalf("rollback dejo clave: count=%d err=%v", keys, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE order_id = $1`, retryID).Scan(&outbox); err != nil || outbox != 0 {
		t.Fatalf("rollback dejo evento outbox: count=%d err=%v", outbox, err)
	}
	brokenInput.MessageID = "88888888-8888-4888-8888-888888888888"
	if _, err := repo.CreateIdempotent(ctx, brokenInput); err != nil {
		t.Fatalf("la clave liberada debe poder reintentarse: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE order_id = $1`, retryID).Scan(&outbox); err != nil || outbox != 1 {
		t.Fatalf("creacion debe dejar un evento outbox: count=%d err=%v", outbox, err)
	}
}

func idempotentInput(key, fingerprint string, order entity.Order, snapshot dto.CreateOrderOutput, eventID, messageID string) port.IdempotentOrder {
	return port.IdempotentOrder{
		Key: key, Fingerprint: fingerprint, Order: order, Snapshot: snapshot, MessageID: messageID,
		Event: event.NewOrderCreated(eventID, order.ID, order.ID, order.CreatedAt),
	}
}
