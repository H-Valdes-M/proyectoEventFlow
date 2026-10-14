package httputil

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"eventflow/pkg/apperrors"
)

// ErrorBody es el contrato estable de error expuesto por la API.
type ErrorBody struct {
	Type    apperrors.Type `json:"type"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// ErrorResponse conserva el error, el instante UTC y la correlación de la
// petición sin incluir causas privadas.
type ErrorResponse struct {
	Error     ErrorBody `json:"error"`
	Timestamp time.Time `json:"timestamp"`
	RequestID string    `json:"request_id"`
}

// MapErrorToHTTP convierte un error de dominio o aplicación al contrato HTTP.
// now se recibe para hacer el resultado determinista en pruebas.
func MapErrorToHTTP(err error, requestID string, now time.Time) (int, ErrorResponse) {
	var typed *apperrors.Error
	if !errors.As(err, &typed) {
		return internalResponse(requestID, now)
	}

	status, known := statusFor(typed.Type)
	if !known || typed.Type == apperrors.InternalError || typed.Type == apperrors.ServiceUnavailable {
		return genericResponse(status, typed.Type, typed.Code, requestID, now)
	}

	body := ErrorBody{
		Type:    typed.Type,
		Code:    typed.Code,
		Message: typed.Message,
	}
	if typed.Type == apperrors.ValidationError {
		body.Details = copyDetails(typed.Details)
	}
	return status, ErrorResponse{
		Error:     body,
		Timestamp: now.UTC(),
		RequestID: requestID,
	}
}

// WriteError serializa el resultado del mapeo como JSON. Los handlers futuros
// pueden delegar aquí sin exponer err.Error() directamente al cliente.
func WriteError(w http.ResponseWriter, err error, requestID string, now time.Time) error {
	status, response := MapErrorToHTTP(err, requestID, now)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(response)
}

func statusFor(errorType apperrors.Type) (int, bool) {
	switch errorType {
	case apperrors.BadRequest:
		return http.StatusBadRequest, true
	case apperrors.ValidationError:
		return http.StatusUnprocessableEntity, true
	case apperrors.NotFound:
		return http.StatusNotFound, true
	case apperrors.Conflict:
		return http.StatusConflict, true
	case apperrors.PayloadTooLarge:
		return http.StatusRequestEntityTooLarge, true
	case apperrors.UnsupportedMediaType:
		return http.StatusUnsupportedMediaType, true
	case apperrors.ServiceUnavailable:
		return http.StatusServiceUnavailable, true
	case apperrors.InternalError:
		return http.StatusInternalServerError, true
	default:
		return http.StatusInternalServerError, false
	}
}

func internalResponse(requestID string, now time.Time) (int, ErrorResponse) {
	return genericResponse(http.StatusInternalServerError, apperrors.InternalError, "INTERNAL_ERROR", requestID, now)
}

func genericResponse(status int, errorType apperrors.Type, code, requestID string, now time.Time) (int, ErrorResponse) {
	message := "Ocurrio un error interno"
	if errorType == apperrors.ServiceUnavailable {
		message = "El servicio no esta disponible temporalmente"
	}
	if code == "" {
		code = string(errorType)
	}
	return status, ErrorResponse{
		Error: ErrorBody{
			Type:    errorType,
			Code:    code,
			Message: message,
		},
		Timestamp: now.UTC(),
		RequestID: requestID,
	}
}

func copyDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}
	copy := make(map[string]any, len(details))
	for key, value := range details {
		copy[key] = value
	}
	return copy
}
