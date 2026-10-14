package httputil

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"eventflow/pkg/apperrors"
)

func TestMapErrorToHTTPCategorias(t *testing.T) {
	now := time.Date(2026, 10, 12, 17, 0, 0, 0, time.FixedZone("UTC-3", -3*60*60))
	cases := []struct {
		name      string
		errorType apperrors.Type
		status    int
	}{
		{name: "formato", errorType: apperrors.BadRequest, status: http.StatusBadRequest},
		{name: "negocio", errorType: apperrors.ValidationError, status: http.StatusUnprocessableEntity},
		{name: "ausencia", errorType: apperrors.NotFound, status: http.StatusNotFound},
		{name: "conflicto", errorType: apperrors.Conflict, status: http.StatusConflict},
		{name: "tamano", errorType: apperrors.PayloadTooLarge, status: http.StatusRequestEntityTooLarge},
		{name: "content type", errorType: apperrors.UnsupportedMediaType, status: http.StatusUnsupportedMediaType},
		{name: "dependencia", errorType: apperrors.ServiceUnavailable, status: http.StatusServiceUnavailable},
		{name: "interno", errorType: apperrors.InternalError, status: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, response := MapErrorToHTTP(apperrors.New(tc.errorType, "TEST_CODE", "Mensaje publico"), "req-123", now)
			if status != tc.status {
				t.Fatalf("status = %d, se esperaba %d", status, tc.status)
			}
			if response.Error.Type != tc.errorType || response.RequestID != "req-123" {
				t.Fatalf("respuesta inesperada: %+v", response)
			}
			if response.Timestamp.Location() != time.UTC || !response.Timestamp.Equal(now.UTC()) {
				t.Fatalf("timestamp debe estar en UTC: %s", response.Timestamp)
			}
		})
	}
}

func TestMapErrorToHTTPReconoceErrorEnvueltoYDetails(t *testing.T) {
	typed := apperrors.New(apperrors.ValidationError, "ORDER_VALIDATION", "La cantidad debe ser positiva")
	typed.Details = map[string]any{"field": "items[0].quantity"}
	err := fmtWrap(typed)

	status, response := MapErrorToHTTP(err, "req-456", time.Now())
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, se esperaba 422", status)
	}
	if response.Error.Details["field"] != "items[0].quantity" {
		t.Fatalf("details inesperados: %v", response.Error.Details)
	}
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Fatal("el error envuelto debe conservar su categoria")
	}
}

func TestMapErrorToHTTPNoFiltraCausasInternas(t *testing.T) {
	privateCause := errors.New("postgres://admin:clave@db/eventflow SELECT secreto")
	err := apperrors.Wrap(privateCause, apperrors.ServiceUnavailable, "DB_UNAVAILABLE", "No usar este mensaje")
	_, response := MapErrorToHTTP(err, "req-789", time.Now())

	encoded, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		t.Fatalf("serializar respuesta: %v", marshalErr)
	}
	if strings.Contains(string(encoded), "clave") || strings.Contains(string(encoded), "postgres://") || strings.Contains(string(encoded), "No usar este mensaje") {
		t.Fatalf("la respuesta filtro una causa interna: %s", encoded)
	}
	if response.Error.Message != "El servicio no esta disponible temporalmente" {
		t.Fatalf("mensaje 503 inesperado: %q", response.Error.Message)
	}
}

func TestMapErrorToHTTPDesconocidoEsGenerico(t *testing.T) {
	_, response := MapErrorToHTTP(errors.New("fallo sql con password=secreta"), "req-000", time.Now())
	if response.Error.Type != apperrors.InternalError || response.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("error desconocido debe ser interno: %+v", response.Error)
	}
	if strings.Contains(response.Error.Message, "password") {
		t.Fatalf("mensaje interno no debe filtrar la causa: %q", response.Error.Message)
	}
}

func TestWriteErrorProduceJSONUniforme(t *testing.T) {
	recorder := httptest.NewRecorder()
	now := time.Date(2026, 10, 12, 17, 0, 0, 0, time.UTC)
	err := apperrors.New(apperrors.BadRequest, "INVALID_JSON", "El JSON no es valido")
	if writeErr := WriteError(recorder, err, "req-http", now); writeErr != nil {
		t.Fatalf("escribir respuesta: %v", writeErr)
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, se esperaba 400", recorder.Code)
	}
	if recorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("content type inesperado: %q", recorder.Header().Get("Content-Type"))
	}
	var response ErrorResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("respuesta no es JSON valido: %v", err)
	}
	if response.Error.Code != "INVALID_JSON" || response.RequestID != "req-http" {
		t.Fatalf("respuesta inesperada: %+v", response)
	}
}

func fmtWrap(err error) error {
	return fmt.Errorf("caso de uso: %w", err)
}
