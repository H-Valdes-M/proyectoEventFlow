package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eventflow/internal/order/application/dto"
	"eventflow/internal/order/presentation/handler"
	"eventflow/pkg/apperrors"
	"eventflow/pkg/metrics"
	"github.com/gin-gonic/gin"
)

func TestHealthzDevuelveProcesoVivo(t *testing.T) {
	router := New(nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, se esperaba %d", recorder.Code, http.StatusOK)
	}
	if recorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("content type inesperado: %q", recorder.Header().Get("Content-Type"))
	}
	if strings.TrimSpace(recorder.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("body inesperado: %s", recorder.Body.String())
	}
}

func TestCorrelacionValidaOGeneraYMetricasNoIncluyenIDs(t *testing.T) {
	registry := metrics.NewRegistry()
	router := New(nil, WithMetrics(registry, registry.Handler(func(*http.Request) (metrics.DurableStats, error) { return metrics.DurableStats{}, nil })))
	valid := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	valid.Header.Set("X-Request-ID", "req.demo_1-ABC")
	validResponse := httptest.NewRecorder()
	router.ServeHTTP(validResponse, valid)
	if got := validResponse.Header().Get("X-Request-ID"); got != "req.demo_1-ABC" {
		t.Fatalf("ID valido = %q", got)
	}
	invalid := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	invalid.Header.Set("X-Request-ID", "invalido con espacio")
	invalidResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidResponse, invalid)
	if got := invalidResponse.Header().Get("X-Request-ID"); len(got) != 36 || got == invalid.Header.Get("X-Request-ID") {
		t.Fatalf("ID generado invalido = %q", got)
	}
	metricResponse := httptest.NewRecorder()
	router.ServeHTTP(metricResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if strings.Contains(metricResponse.Body.String(), "req.demo_1-ABC") || !strings.Contains(metricResponse.Body.String(), "eventflow_http_requests_total") {
		t.Fatalf("metricas inseguras o ausentes: %s", metricResponse.Body.String())
	}
}

func TestRecoveryEscribeErrorSanitizadoUnaVez(t *testing.T) {
	router := New(nil)
	router.GET("/panic", func(*gin.Context) { panic("postgres://admin:secreto@db") })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))
	body := recorder.Body.String()

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, se esperaba %d", recorder.Code, http.StatusInternalServerError)
	}
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&response); err != nil {
		t.Fatalf("decodificar respuesta: %v", err)
	}
	if response.Error.Code != "INTERNAL_ERROR" || strings.Contains(response.Error.Message, "secreto") {
		t.Fatalf("respuesta de panic no sanitizada: %+v", response.Error)
	}
	if strings.Count(body, `{"error":`) != 1 {
		t.Fatalf("el recovery debe escribir una sola respuesta: %s", body)
	}
}

func TestMiddlewareDeErroresSanitizaErrorDelHandler(t *testing.T) {
	router := New(nil)
	router.GET("/error", func(c *gin.Context) {
		_ = c.Error(errors.New("password=secreta"))
		c.Abort()
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/error", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, se esperaba %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "secreta") {
		t.Fatalf("la respuesta filtro el error interno: %s", recorder.Body.String())
	}
}

func TestRutasPedidosMapeanErroresDeFormatoYNegocio(t *testing.T) {
	create := &createExecutorDouble{err: apperrors.New(apperrors.ValidationError, "ORDER_VALIDATION", "invalido")}
	get := &getExecutorDouble{err: apperrors.New(apperrors.NotFound, "ORDER_NOT_FOUND", "ausente")}
	router := New(nil, WithOrderHandler(handler.NewOrderHandler(create, get)))
	cases := []struct {
		name, method, path, contentType, body string
		status                                int
	}{
		{"tipo no soportado", http.MethodPost, "/orders", "text/plain", "x", http.StatusUnsupportedMediaType},
		{"campo desconocido", http.MethodPost, "/orders", "application/json", `{"customer_id":"cliente","items":[],"extra":true}`, http.StatusBadRequest},
		{"regla de negocio", http.MethodPost, "/orders", "application/json", `{"customer_id":"cliente","items":[]}`, http.StatusUnprocessableEntity},
		{"id invalido", http.MethodGet, "/orders/invalido", "", "", http.StatusBadRequest},
		{"ausente", http.MethodGet, "/orders/88888888-8888-4888-8888-888888888888", "", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.contentType != "" {
				request.Header.Set("Content-Type", tc.contentType)
			}
			if tc.method == http.MethodPost {
				request.Header.Set("Idempotency-Key", "rutas-pedido-1")
			}
			router.ServeHTTP(recorder, request)
			if recorder.Code != tc.status {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestReadinessNoAfectaLiveness(t *testing.T) {
	router := New(nil, WithReadiness(readinessDouble{err: errors.New("DB caída")}))
	for _, tc := range []struct {
		path   string
		status int
	}{{"/healthz", http.StatusOK}, {"/readyz", http.StatusServiceUnavailable}} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if recorder.Code != tc.status {
			t.Fatalf("%s status=%d", tc.path, recorder.Code)
		}
	}
}

type createExecutorDouble struct{ err error }

func (d *createExecutorDouble) Execute(context.Context, dto.CreateOrderInput, string) (dto.CreateOrderOutput, error) {
	return dto.CreateOrderOutput{}, d.err
}

type getExecutorDouble struct{ err error }

func (d *getExecutorDouble) Execute(context.Context, string) (dto.GetOrderOutput, error) {
	return dto.GetOrderOutput{}, d.err
}

type readinessDouble struct{ err error }

func (d readinessDouble) Execute(context.Context) error { return d.err }
