package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"eventflow/internal/order/application/dto"
	"github.com/gin-gonic/gin"
)

func TestCreateResponde201YLocation(t *testing.T) {
	h := NewOrderHandler(createDouble{output: dto.CreateOrderOutput{ID: "88888888-8888-4888-8888-888888888888", Status: "pending", TotalMinor: 3000}}, &getDouble{})
	router := gin.New()
	router.POST("/orders", h.Create)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{"customer_id":"cliente","items":[{"product_id":"producto","quantity":2,"unit_price_minor":1500}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "crear-pedido-1")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || recorder.Header().Get("Location") != "/orders/88888888-8888-4888-8888-888888888888" {
		t.Fatalf("respuesta inesperada: status=%d location=%q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestCreateRechazaFormatoAntesDelCasoDeUso(t *testing.T) {
	cases := []struct{ name, contentType, body string }{
		{"tipo no soportado", "text/plain", "x"},
		{"campo desconocido", "application/json", `{"customer_id":"cliente","items":[],"extra":true}`},
		{"dos valores", "application/json", `{} {}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			create := &createSpy{}
			h := NewOrderHandler(create, &getDouble{})
			router := gin.New()
			router.POST("/orders", h.Create)
			request := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", tc.contentType)
			request.Header.Set("Idempotency-Key", "crear-pedido-1")
			router.ServeHTTP(httptest.NewRecorder(), request)
			if create.calls != 0 {
				t.Fatal("un formato invalido no debe invocar el caso de uso")
			}
		})
	}
}

func TestValidateIdempotencyKey(t *testing.T) {
	for _, key := range []string{"", "con espacio", "tab\t", string(make([]byte, 129))} {
		if _, err := validateIdempotencyKey(key); err == nil {
			t.Fatalf("la clave %q debio rechazarse", key)
		}
	}
	if got, err := validateIdempotencyKey("pedido-ABC_123"); err != nil || got != "pedido-ABC_123" {
		t.Fatalf("clave valida inesperada: %q %v", got, err)
	}
}

func TestGetByIDDistingueIDInvalidoYRespuesta(t *testing.T) {
	get := &getDouble{output: dto.GetOrderOutput{ID: "88888888-8888-4888-8888-888888888888", CustomerID: "cliente", Status: "pending", CreatedAt: time.Now(), UpdatedAt: time.Now()}}
	h := NewOrderHandler(createDouble{}, get)
	router := gin.New()
	router.GET("/orders/:id", h.GetByID)
	invalid := httptest.NewRecorder()
	router.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/orders/invalido", nil))
	if get.calls != 0 {
		t.Fatal("un UUID invalido no debe consultar el caso de uso")
	}
	valid := httptest.NewRecorder()
	router.ServeHTTP(valid, httptest.NewRequest(http.MethodGet, "/orders/88888888-8888-4888-8888-888888888888", nil))
	if valid.Code != http.StatusOK || get.calls != 1 || !strings.Contains(valid.Body.String(), `"customer_id":"cliente"`) {
		t.Fatalf("pedido HTTP inesperado: %d %s", valid.Code, valid.Body.String())
	}
}

type createDouble struct {
	output dto.CreateOrderOutput
	err    error
}

func (d createDouble) Execute(context.Context, dto.CreateOrderInput, string) (dto.CreateOrderOutput, error) {
	return d.output, d.err
}

type createSpy struct{ calls int }

func (d *createSpy) Execute(context.Context, dto.CreateOrderInput, string) (dto.CreateOrderOutput, error) {
	d.calls++
	return dto.CreateOrderOutput{}, errors.New("no debe ejecutarse")
}

type getDouble struct {
	output dto.GetOrderOutput
	err    error
	calls  int
}

func (d *getDouble) Execute(context.Context, string) (dto.GetOrderOutput, error) {
	d.calls++
	return d.output, d.err
}
