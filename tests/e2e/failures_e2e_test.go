//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"eventflow/cmd/app"
	"eventflow/cmd/config"
	"eventflow/cmd/container"
	"eventflow/cmd/routes"
	"eventflow/internal/order/application/port"
	"eventflow/internal/order/infrastructure/messaging"
	"eventflow/pkg/database"
	"eventflow/pkg/pubsub"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestE2ERespuestaPerdidaReproduceSnapshot(t *testing.T) {
	ctx, cancel := e2eContext(t, 10*time.Second)
	defer cancel()
	server, db, orderHandler := newE2EAPI(t, ctx)
	defer server.Close()
	defer db.Close()
	pool := postgresPool(t, db)
	key := unique("mt40")
	release := make(chan struct{})
	committed := make(chan struct{})
	var barrier sync.Once
	orderHandler.SetTestAfterCreateCommit(func(context.Context) {
		barrier.Do(func() {
			close(committed)
			<-release
		})
	})

	requestCtx, requestCancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer requestCancel()
	request := newOrderRequest(t, requestCtx, server.URL, key, "cliente-"+key)
	result := make(chan error, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		if response != nil {
			response.Body.Close()
		}
		result <- err
	}()
	select {
	case <-committed:
	case <-ctx.Done():
		t.Fatal("no se alcanzó la barrera posterior al commit")
	}
	if err := <-result; err == nil {
		t.Fatal("el cliente debía perder la respuesta tras su plazo")
	}
	close(release)

	replay := postOrder(t, context.Background(), server.URL, key, "cliente-"+key)
	defer replay.Body.Close()
	if replay.StatusCode != http.StatusCreated {
		t.Fatalf("replay status=%d", replay.StatusCode)
	}
	body, _ := io.ReadAll(replay.Body)
	orderID := orderID(t, body)
	defer cleanupOrder(t, pool, key, orderID)
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM orders WHERE id = $1`, 1, orderID)
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM outbox_events WHERE order_id = $1`, 1, orderID)
}

func TestE2EBrokerDetenidoSeRecuperaSinReenviarPOST(t *testing.T) {
	ctx, cancel := e2eContext(t, 30*time.Second)
	defer cancel()
	server, db, _ := newE2EAPI(t, ctx)
	defer server.Close()
	defer db.Close()
	pool := postgresPool(t, db)
	worker, stopWorker := startWorker(t, ctx)
	defer stopWorker()
	compose(t, "stop", "rabbitmq")
	defer compose(t, "start", "rabbitmq")

	key := unique("mt41")
	response := postOrder(t, ctx, server.URL, key, "cliente-"+key)
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("crear con broker detenido status=%d", response.StatusCode)
	}
	orderID := orderID(t, body)
	defer cleanupOrder(t, pool, key, orderID)
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM outbox_events WHERE order_id = $1 AND published_at IS NULL`, 1, orderID)
	assertStatus(t, ctx, pool, orderID, "pending")
	_ = worker
	compose(t, "start", "rabbitmq")
	waitFor(t, ctx, func() bool {
		var status string
		err := pool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status)
		return err == nil && status == "processed"
	})
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM notifications WHERE order_id = $1`, 1, orderID)
}

func TestE2ECaidaEntreCommitYAckRedeliveryDeduplicada(t *testing.T) {
	ctx, cancel := e2eContext(t, 20*time.Second)
	defer cancel()
	server, db, _ := newE2EAPI(t, ctx)
	defer server.Close()
	defer db.Close()
	pool := postgresPool(t, db)
	key := unique("mt42")
	response := postOrder(t, ctx, server.URL, key, "cliente-"+key)
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("crear pedido status=%d", response.StatusCode)
	}
	orderID := orderID(t, body)
	defer cleanupOrder(t, pool, key, orderID)

	crashed := make(chan struct{})
	crash := errors.New("fallo e2e antes de ACK")
	restore := pubsub.SetTestBeforeAck(func() error {
		close(crashed)
		return crash
	})
	firstCtx, stopFirst := context.WithCancel(ctx)
	first, firstDone := runWorker(t, firstCtx)
	select {
	case <-crashed:
		stopFirst()
	case <-ctx.Done():
		t.Fatal("no se alcanzó el failpoint entre commit y ACK")
	}
	if err := <-firstDone; err != nil {
		t.Fatalf("worker interrumpido: %v", err)
	}
	_ = first.Shutdown(context.Background())
	restore()

	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM notifications WHERE order_id = $1`, 1, orderID)
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM event_processing WHERE order_id = $1 AND status = 'completed'`, 1, orderID)
	redelivered := make(chan struct{})
	restoreSecond := pubsub.SetTestBeforeAck(func() error {
		close(redelivered)
		return nil
	})
	_, stopSecond := startWorker(t, ctx)
	defer stopSecond()
	select {
	case <-redelivered:
	case <-ctx.Done():
		t.Fatal("no se confirmó la redelivery tras reiniciar el worker")
	}
	restoreSecond()
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM notifications WHERE order_id = $1`, 1, orderID)
}

func TestE2ECuatroFallosTransitoriosReinicioYDuplicadoTerminan(t *testing.T) {
	ctx, cancel := e2eContext(t, 35*time.Second)
	defer cancel()
	server, db, _ := newE2EAPI(t, ctx)
	defer server.Close()
	defer db.Close()
	pool := postgresPool(t, db)
	key := unique("mt43")
	response := postOrder(t, ctx, server.URL, key, "cliente-"+key)
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("crear pedido status=%d", response.StatusCode)
	}
	orderID := orderID(t, body)
	defer cleanupOrder(t, pool, key, orderID)
	var eventID string
	var payload []byte
	if err := pool.QueryRow(ctx, `SELECT event_id, payload FROM outbox_events WHERE order_id = $1`, orderID).Scan(&eventID, &payload); err != nil {
		t.Fatal("leer evento inicial: ", err)
	}
	var mu sync.Mutex
	failures := make(map[int]int)
	restore := messaging.SetTestBeforeProcess(func(delivery pubsub.Delivery) error {
		if delivery.EventID != eventID {
			return nil
		}
		var status string
		var attempts int
		err := pool.QueryRow(context.Background(), `SELECT status, attempts FROM event_processing WHERE consumer_name = 'orders-worker' AND event_id = $1`, eventID).Scan(&status, &attempts)
		inject := errors.Is(err, pgx.ErrNoRows) && delivery.Attempt == 1 || err == nil && status == "retry_scheduled" && attempts == delivery.Attempt-1
		if !inject {
			return nil
		}
		mu.Lock()
		failures[delivery.Attempt]++
		mu.Unlock()
		return port.ErrTransientProcessing
	})
	defer restore()
	first, stopFirst := startWorker(t, ctx)
	waitFor(t, ctx, func() bool {
		var attempts int
		var status string
		err := pool.QueryRow(ctx, `SELECT status, attempts FROM event_processing WHERE consumer_name = 'orders-worker' AND event_id = $1`, eventID).Scan(&status, &attempts)
		return err == nil && status == "retry_scheduled" && attempts == 1
	})
	publisher, err := pubsub.NewRabbitMQProvider(ctx, workerConfig(t).AMQPURL)
	if err != nil {
		t.Fatal("conectar duplicado e2e: ", err)
	}
	if err := publisher.PublishConfirmed(ctx, pubsub.Message{MessageID: uniqueUUID(t), EventID: eventID, Type: "order.created", Attempt: 1, Payload: payload}); err != nil {
		publisher.Close()
		t.Fatal("publicar duplicado: ", err)
	}
	publisher.Close()
	stopFirst()
	_ = first
	second, stopSecond := startWorker(t, ctx)
	defer stopSecond()
	_ = second
	waitFor(t, ctx, func() bool {
		var status string
		err := pool.QueryRow(ctx, `SELECT status FROM event_processing WHERE consumer_name = 'orders-worker' AND event_id = $1`, eventID).Scan(&status)
		return err == nil && status == "failed"
	})
	mu.Lock()
	defer mu.Unlock()
	for attempt := 1; attempt <= 4; attempt++ {
		if failures[attempt] != 1 {
			t.Fatalf("fallos inyectados del intento %d: got=%d want=1", attempt, failures[attempt])
		}
	}
	assertStatus(t, ctx, pool, orderID, "failed")
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM failed_events WHERE event_id = $1`, 1, eventID)
	var reason string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT attempts, last_error_sanitized FROM event_processing WHERE event_id = $1`, eventID).Scan(&attempts, &reason); err != nil || attempts != 4 || reason != "fallo de negocio agotado" {
		t.Fatalf("estado terminal inesperado: attempts=%d reason=%q err=%v", attempts, reason, err)
	}
	for attempt, want := range map[int]float64{2: 1, 3: 2, 4: 4} {
		var seconds float64
		if err := pool.QueryRow(ctx, `SELECT EXTRACT(EPOCH FROM available_at - created_at) FROM outbox_events WHERE event_id = $1 AND attempt = $2`, eventID, attempt).Scan(&seconds); err != nil || seconds < want-0.01 || seconds > want+0.01 {
			t.Fatalf("demora intento %d: got=%f want=%f err=%v", attempt, seconds, want, err)
		}
	}
}

func e2eContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	if os.Getenv("EVENTFLOW_E2E") != "1" {
		t.Skip("definir EVENTFLOW_E2E=1 para ejecutar demostraciones con PostgreSQL y RabbitMQ reales")
	}
	t.Chdir(filepath.Join("..", ".."))
	return context.WithTimeout(context.Background(), timeout)
}

func newE2EAPI(t *testing.T, ctx context.Context) (*httptest.Server, database.Database, interface{ SetTestAfterCreateCommit(func(context.Context)) }) {
	t.Helper()
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.NewPostgresDatabase(ctx, database.PostgresConfig{Host: cfg.DBHost, Port: cfg.DBPort, User: cfg.DBUser, Password: cfg.DBPassword, Database: cfg.DBName, SSLMode: cfg.DBSSLMode})
	if err != nil {
		t.Fatal(err)
	}
	h, err := container.NewOrderHandler(db)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	return httptest.NewServer(routes.New(nil, routes.WithOrderHandler(h))), db, h
}

func workerConfig(t *testing.T) *config.WorkerConfig {
	cfg, err := config.LoadWorkerConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.OutboxPollInterval = 100 * time.Millisecond
	cfg.OutboxBatchSize = 100
	return cfg
}
func runWorker(t *testing.T, ctx context.Context) (*app.WorkerResources, <-chan error) {
	t.Helper()
	cfg := workerConfig(t)
	apiCfg, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.NewPostgresDatabase(ctx, database.PostgresConfig{Host: apiCfg.DBHost, Port: apiCfg.DBPort, User: apiCfg.DBUser, Password: apiCfg.DBPassword, Database: apiCfg.DBName, SSLMode: apiCfg.DBSSLMode})
	if err != nil {
		t.Fatal(err)
	}
	worker := &app.WorkerResources{Config: cfg, Database: db}
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	return worker, done
}
func startWorker(t *testing.T, parent context.Context) (*app.WorkerResources, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(parent)
	worker, done := runWorker(t, ctx)
	return worker, func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("detener worker: %v", err)
		}
		_ = worker.Shutdown(context.Background())
	}
}
func postgresPool(t *testing.T, db database.Database) *pgxpool.Pool {
	t.Helper()
	pool, err := database.AsPostgresPool(db)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}
func unique(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()) }
func uniqueUUID(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%08x-0000-4000-8000-%012x", time.Now().UnixNano()&0xffffffff, time.Now().UnixNano()&0xffffffffffff)
}
func newOrderRequest(t *testing.T, ctx context.Context, url, key, customer string) *http.Request {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/orders", bytes.NewBufferString(fmt.Sprintf(`{"customer_id":%q,"items":[{"product_id":"producto-e2e","quantity":1,"unit_price_minor":100}]}`, customer)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	return request
}
func postOrder(t *testing.T, ctx context.Context, url, key, customer string) *http.Response {
	t.Helper()
	response, err := http.DefaultClient.Do(newOrderRequest(t, ctx, url, key, customer))
	if err != nil {
		t.Fatal(err)
	}
	return response
}
func orderID(t *testing.T, body []byte) string {
	t.Helper()
	var output struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &output); err != nil || output.ID == "" {
		t.Fatalf("respuesta pedido invalida: %s err=%v", body, err)
	}
	return output.ID
}
func assertCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, want int, args ...any) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, query, args...).Scan(&got); err != nil || got != want {
		t.Fatalf("conteo got=%d want=%d err=%v", got, want, err)
	}
}
func assertStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderID, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&got); err != nil || got != want {
		t.Fatalf("estado got=%q want=%q err=%v", got, want, err)
	}
}
func waitFor(t *testing.T, ctx context.Context, ready func() bool) {
	t.Helper()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ready() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("se agotó la espera e2e: ", ctx.Err())
		case <-ticker.C:
		}
	}
}
func cleanupOrder(t *testing.T, pool *pgxpool.Pool, key, orderID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, cleanup := range []struct {
		query string
		args  []any
	}{
		{`DELETE FROM failed_events WHERE order_id = $1`, []any{orderID}},
		{`DELETE FROM notifications WHERE order_id = $1`, []any{orderID}},
		{`DELETE FROM event_processing WHERE order_id = $1`, []any{orderID}},
		{`DELETE FROM outbox_events WHERE order_id = $1`, []any{orderID}},
		{`DELETE FROM idempotency_keys WHERE idempotency_key = $1`, []any{key}},
		{`DELETE FROM orders WHERE id = $1`, []any{orderID}},
	} {
		if _, err := pool.Exec(ctx, cleanup.query, cleanup.args...); err != nil {
			t.Errorf("limpiar e2e: %v", err)
		}
	}
}
func compose(t *testing.T, args ...string) {
	t.Helper()
	command := exec.Command("docker", append([]string{"compose", "--env-file", ".env.dev"}, args...)...)
	command.Dir = "."
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("docker compose %v: %v: %s", args, err, output)
	}
}
