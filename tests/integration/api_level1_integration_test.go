package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"eventflow/cmd/app"
	"eventflow/cmd/config"
	"eventflow/cmd/container"
	"eventflow/cmd/routes"
	"eventflow/pkg/database"
	"eventflow/pkg/pubsub"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/amqp091-go"
)

func TestAPINivel1PersisteTrasReinicio(t *testing.T) {
	if os.Getenv("EVENTFLOW_PG_INTEGRATION") != "1" {
		t.Skip("definir EVENTFLOW_PG_INTEGRATION=1 para ejecutar contra PostgreSQL real")
	}
	t.Chdir(filepath.Join("..", ".."))
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal("cargar configuracion: ", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	customerID := fmt.Sprintf("mt21-%d", time.Now().UnixNano())
	key := "mt21-" + customerID

	server, db := newAPIServer(t, ctx, cfg)
	var order struct {
		ID string `json:"id"`
	}
	response := postJSON(t, server.URL+"/orders", fmt.Sprintf(`{"customer_id":%q,"items":[{"product_id":"producto-mt21","quantity":2,"unit_price_minor":1500}]}`, customerID), key)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("crear pedido status=%d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(&order); err != nil || order.ID == "" {
		t.Fatalf("decodificar pedido creado: id=%q err=%v", order.ID, err)
	}
	response.Body.Close()
	assertOrder(t, server.URL+"/orders/"+order.ID, customerID)
	server.Close()
	db.Close()

	server, db = newAPIServer(t, ctx, cfg)
	defer server.Close()
	defer db.Close()
	defer deleteOrder(t, ctx, db, order.ID)
	defer deleteIdempotencyKey(t, ctx, db, key)
	assertOrder(t, server.URL+"/orders/"+order.ID, customerID)

	response = postJSON(t, server.URL+"/orders", `{"customer_id":"","items":[]}`, key+"-invalid")
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("pedido invalido status=%d", response.StatusCode)
	}
}

func TestAPICreaPedidoYWorkerPublicaRelay(t *testing.T) {
	if os.Getenv("EVENTFLOW_MESSAGING_INTEGRATION") != "1" {
		t.Skip("definir EVENTFLOW_MESSAGING_INTEGRATION=1 para ejecutar contra PostgreSQL y RabbitMQ reales")
	}
	t.Chdir(filepath.Join("..", ".."))
	apiConfig, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal("cargar configuracion API: ", err)
	}
	workerConfig, err := config.LoadWorkerConfig()
	if err != nil {
		t.Fatal("cargar configuracion worker: ", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server, db := newAPIServer(t, ctx, apiConfig)
	defer server.Close()
	defer db.Close()
	key := fmt.Sprintf("mt31-%d", time.Now().UnixNano())
	response := postJSON(t, server.URL+"/orders", fmt.Sprintf(`{"customer_id":%q,"items":[{"product_id":"producto-mt31","quantity":1,"unit_price_minor":100}]}`, "cliente-"+key), key)
	if response.StatusCode != http.StatusCreated {
		response.Body.Close()
		t.Fatalf("crear pedido status=%d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal("leer respuesta de creacion: ", err)
	}
	orderID := responseOrderID(t, body)
	defer deleteOrder(t, ctx, db, orderID)
	defer deleteIdempotencyKey(t, ctx, db, key)

	connection, err := amqp091.Dial(workerConfig.AMQPURL)
	if err != nil {
		t.Fatal("conectar consumidor de evidencia RabbitMQ: ", err)
	}
	defer connection.Close()
	channel, err := connection.Channel()
	if err != nil {
		t.Fatal("abrir canal consumidor de evidencia: ", err)
	}
	defer channel.Close()
	if _, err := channel.QueueDeclare(pubsub.Queue, true, false, false, false, nil); err != nil {
		t.Fatal("declarar cola de evidencia: ", err)
	}
	deliveries, err := channel.Consume(pubsub.Queue, "mt31-evidence", true, false, false, false, nil)
	if err != nil {
		t.Fatal("consumir cola de evidencia: ", err)
	}

	workerConfig.OutboxBatchSize = 100
	workerConfig.OutboxPollInterval = time.Hour
	workerDB, err := database.NewPostgresDatabase(ctx, database.PostgresConfig{
		Host: apiConfig.DBHost, Port: apiConfig.DBPort, User: apiConfig.DBUser, Password: apiConfig.DBPassword, Database: apiConfig.DBName, SSLMode: apiConfig.DBSSLMode,
	})
	if err != nil {
		t.Fatal("conectar PostgreSQL para worker: ", err)
	}
	resources := &app.WorkerResources{Config: workerConfig, Database: workerDB}
	workerDone := make(chan error, 1)
	workerCtx, stopWorker := context.WithCancel(ctx)
	go func() { workerDone <- resources.Run(workerCtx) }()
	defer func() {
		stopWorker()
		if err := <-workerDone; err != nil {
			t.Errorf("detener worker: %v", err)
		}
		_ = resources.Shutdown(context.Background())
	}()

	for {
		select {
		case delivery := <-deliveries:
			var event struct {
				OrderID string `json:"order_id"`
			}
			if err := json.Unmarshal(delivery.Body, &event); err != nil {
				t.Fatal("decodificar evento publicado: ", err)
			}
			if event.OrderID == orderID {
				return
			}
		case <-ctx.Done():
			t.Fatal("el worker no publicó el evento creado por la API")
		}
	}
}

func TestAPIIdempotenciaReproduceRespuestaYDetectaConflicto(t *testing.T) {
	if os.Getenv("EVENTFLOW_PG_INTEGRATION") != "1" {
		t.Skip("definir EVENTFLOW_PG_INTEGRATION=1 para ejecutar contra PostgreSQL real")
	}
	t.Chdir(filepath.Join("..", ".."))
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal("cargar configuracion: ", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	key := fmt.Sprintf("mt25-%d", time.Now().UnixNano())
	customerID := "cliente-" + key
	server, db := newAPIServer(t, ctx, cfg)
	defer server.Close()
	defer db.Close()

	first := postJSON(t, server.URL+"/orders", fmt.Sprintf(`{"customer_id":%q,"items":[{"product_id":"producto-b","quantity":2,"unit_price_minor":1500},{"product_id":"producto-a","quantity":1,"unit_price_minor":200}]}`, customerID), key)
	firstBody, err := io.ReadAll(first.Body)
	first.Body.Close()
	if err != nil || first.StatusCode != http.StatusCreated {
		t.Fatalf("primera creacion inesperada: status=%d err=%v body=%s", first.StatusCode, err, firstBody)
	}
	var output struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(firstBody, &output); err != nil || output.ID == "" {
		t.Fatalf("respuesta inicial invalida: id=%q err=%v", output.ID, err)
	}
	defer deleteOrder(t, ctx, db, output.ID)
	defer deleteIdempotencyKey(t, ctx, db, key)

	replay := postJSON(t, server.URL+"/orders", fmt.Sprintf(`{"customer_id":%q,"items":[{"product_id":"producto-a","quantity":1,"unit_price_minor":200},{"product_id":"producto-b","quantity":2,"unit_price_minor":1500}]}`, " "+customerID+" "), key)
	replayBody, err := io.ReadAll(replay.Body)
	replay.Body.Close()
	if err != nil || replay.StatusCode != http.StatusCreated || string(replayBody) != string(firstBody) {
		t.Fatalf("replay inesperado: status=%d err=%v first=%s replay=%s", replay.StatusCode, err, firstBody, replayBody)
	}

	conflict := postJSON(t, server.URL+"/orders", fmt.Sprintf(`{"customer_id":%q,"items":[{"product_id":"producto-a","quantity":1,"unit_price_minor":200}]}`, customerID+"-otro"), key)
	defer conflict.Body.Close()
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("conflicto inesperado: status=%d", conflict.StatusCode)
	}
}

func TestAPIIdempotenciaConcurrente(t *testing.T) {
	if os.Getenv("EVENTFLOW_PG_INTEGRATION") != "1" {
		t.Skip("definir EVENTFLOW_PG_INTEGRATION=1 para ejecutar contra PostgreSQL real")
	}
	t.Chdir(filepath.Join("..", ".."))
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		t.Fatal("cargar configuracion: ", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	key := fmt.Sprintf("mt26-%d", time.Now().UnixNano())
	body := fmt.Sprintf(`{"customer_id":%q,"items":[{"product_id":"producto-mt26","quantity":2,"unit_price_minor":1500}]}`, "cliente-"+key)
	server, db := newAPIServer(t, ctx, cfg)

	responses := concurrentPosts(server.URL+"/orders", body, key, 20)
	var orderID string
	for _, response := range responses {
		if response.err != nil || response.status != http.StatusCreated {
			t.Fatalf("POST concurrente inesperado: status=%d err=%v body=%s", response.status, response.err, response.body)
		}
		id := responseOrderID(t, response.body)
		if orderID == "" {
			orderID = id
		} else if id != orderID {
			t.Fatalf("las respuestas concurrentes no apuntan al mismo pedido: got=%s want=%s", id, orderID)
		}
	}
	pool, err := database.AsPostgresPool(db)
	if err != nil {
		t.Fatal("obtener pool: ", err)
	}
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM idempotency_keys WHERE idempotency_key = $1`, 1, key)
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM orders WHERE id = $1`, 1, orderID)
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM outbox_events WHERE order_id = $1`, 1, orderID)

	server.Close()
	db.Close()
	server, db = newAPIServer(t, ctx, cfg)
	defer server.Close()
	defer db.Close()
	pool, err = database.AsPostgresPool(db)
	if err != nil {
		t.Fatal("obtener pool tras reinicio: ", err)
	}
	replay := postJSON(t, server.URL+"/orders", body, key)
	replayBody, err := io.ReadAll(replay.Body)
	replay.Body.Close()
	if err != nil || replay.StatusCode != http.StatusCreated || responseOrderID(t, replayBody) != orderID {
		t.Fatalf("replay tras reinicio inesperado: status=%d err=%v body=%s", replay.StatusCode, err, replayBody)
	}
	assertCount(t, ctx, pool, `SELECT COUNT(*) FROM outbox_events WHERE order_id = $1`, 1, orderID)

	distinctKeys := []string{key + "-a", key + "-b"}
	distinctResponses := concurrentPosts(server.URL+"/orders", body, distinctKeys[0], 1)
	distinctResponses = append(distinctResponses, concurrentPosts(server.URL+"/orders", body, distinctKeys[1], 1)...)
	distinctIDs := make([]string, 0, len(distinctResponses))
	for _, response := range distinctResponses {
		if response.err != nil || response.status != http.StatusCreated {
			t.Fatalf("POST con clave distinta inesperado: status=%d err=%v", response.status, response.err)
		}
		distinctIDs = append(distinctIDs, responseOrderID(t, response.body))
	}
	if distinctIDs[0] == distinctIDs[1] || distinctIDs[0] == orderID || distinctIDs[1] == orderID {
		t.Fatalf("claves distintas deben crear pedidos distintos: %v", append(distinctIDs, orderID))
	}

	conflictKey := key + "-conflicto"
	conflictResponses := concurrentPostsDifferentBodies(server.URL+"/orders", conflictKey, []string{body, fmt.Sprintf(`{"customer_id":%q,"items":[{"product_id":"producto-mt26","quantity":1,"unit_price_minor":1500}]}`, "cliente-"+key)})
	created, conflicted := 0, 0
	for _, response := range conflictResponses {
		if response.err != nil {
			t.Fatalf("POST de conflicto concurrente: %v", response.err)
		}
		switch response.status {
		case http.StatusCreated:
			created++
			distinctIDs = append(distinctIDs, responseOrderID(t, response.body))
		case http.StatusConflict:
			conflicted++
		default:
			t.Fatalf("status inesperado en conflicto concurrente: %d body=%s", response.status, response.body)
		}
	}
	if created != 1 || conflicted != 1 {
		t.Fatalf("conflicto concurrente debe producir un 201 y un 409: created=%d conflict=%d", created, conflicted)
	}

	keys := append(distinctKeys, key, conflictKey)
	cleanupIdempotentOrders(t, ctx, pool, keys, append(distinctIDs, orderID))
}

func newAPIServer(t *testing.T, ctx context.Context, cfg *config.APIConfig) (*httptest.Server, database.Database) {
	t.Helper()
	db, err := database.NewPostgresDatabase(ctx, database.PostgresConfig{Host: cfg.DBHost, Port: cfg.DBPort, User: cfg.DBUser, Password: cfg.DBPassword, Database: cfg.DBName, SSLMode: cfg.DBSSLMode})
	if err != nil {
		t.Fatal("conectar PostgreSQL: ", err)
	}
	handler, err := container.NewOrderHandler(db)
	if err != nil {
		db.Close()
		t.Fatal("crear handler: ", err)
	}
	return httptest.NewServer(routes.New(nil, routes.WithOrderHandler(handler))), db
}

func postJSON(t *testing.T, url, body, key string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal("crear POST pedido: ", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal("POST pedido: ", err)
	}
	return response
}

func deleteIdempotencyKey(t *testing.T, ctx context.Context, db database.Database, key string) {
	t.Helper()
	pool, err := database.AsPostgresPool(db)
	if err != nil {
		t.Fatal("obtener pool: ", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE idempotency_key = $1`, key); err != nil {
		t.Fatal("limpiar clave de idempotencia: ", err)
	}
}

func assertOrder(t *testing.T, url, customerID string) {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatal("GET pedido: ", err)
	}
	defer response.Body.Close()
	var order struct {
		CustomerID string `json:"customer_id"`
		TotalMinor int64  `json:"total_minor"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&order) != nil || order.CustomerID != customerID || order.TotalMinor != 3000 {
		t.Fatalf("pedido consultado inesperado: status=%d customer_id=%q total=%d", response.StatusCode, order.CustomerID, order.TotalMinor)
	}
}

func deleteOrder(t *testing.T, ctx context.Context, db database.Database, id string) {
	t.Helper()
	pool, err := database.AsPostgresPool(db)
	if err != nil {
		t.Fatal("obtener pool: ", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM outbox_events WHERE order_id = $1`, id); err != nil {
		t.Fatal("limpiar eventos outbox: ", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, id); err != nil {
		t.Fatal("limpiar pedido: ", err)
	}
}

type postResult struct {
	status int
	body   []byte
	err    error
}

func concurrentPosts(url, body, key string, count int) []postResult {
	bodies := make([]string, count)
	for index := range bodies {
		bodies[index] = body
	}
	return concurrentPostsDifferentBodies(url, key, bodies)
}

func concurrentPostsDifferentBodies(url, key string, bodies []string) []postResult {
	results := make([]postResult, len(bodies))
	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	for index, body := range bodies {
		waitGroup.Add(1)
		go func(index int, body string) {
			defer waitGroup.Done()
			<-start
			request, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
			if err != nil {
				results[index].err = err
				return
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", key)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				results[index].err = err
				return
			}
			defer response.Body.Close()
			results[index].status = response.StatusCode
			results[index].body, results[index].err = io.ReadAll(response.Body)
		}(index, body)
	}
	close(start)
	waitGroup.Wait()
	return results
}

func responseOrderID(t *testing.T, body []byte) string {
	t.Helper()
	var output struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &output); err != nil || output.ID == "" {
		t.Fatalf("respuesta de creacion invalida: body=%s err=%v", body, err)
	}
	return output.ID
}

func assertCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, want int, arguments ...any) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, query, arguments...).Scan(&count); err != nil || count != want {
		t.Fatalf("conteo inesperado: query=%s got=%d want=%d err=%v", query, count, want, err)
	}
}

func cleanupIdempotentOrders(t *testing.T, ctx context.Context, pool *pgxpool.Pool, keys, orderIDs []string) {
	t.Helper()
	for _, key := range keys {
		if _, err := pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE idempotency_key = $1`, key); err != nil {
			t.Fatal("limpiar clave de idempotencia: ", err)
		}
	}
	for _, orderID := range orderIDs {
		if _, err := pool.Exec(ctx, `DELETE FROM outbox_events WHERE order_id = $1`, orderID); err != nil {
			t.Fatal("limpiar evento outbox: ", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, orderID); err != nil {
			t.Fatal("limpiar pedido: ", err)
		}
	}
}
