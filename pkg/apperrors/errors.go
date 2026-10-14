package apperrors

import "fmt"

// Type clasifica una condición de negocio o transporte sin depender de su
// mensaje. El adaptador HTTP decide el status a partir de esta categoría.
type Type string

const (
	BadRequest           Type = "BAD_REQUEST"
	ValidationError      Type = "VALIDATION_ERROR"
	NotFound             Type = "NOT_FOUND"
	Conflict             Type = "CONFLICT"
	PayloadTooLarge      Type = "PAYLOAD_TOO_LARGE"
	UnsupportedMediaType Type = "UNSUPPORTED_MEDIA_TYPE"
	ServiceUnavailable   Type = "SERVICE_UNAVAILABLE"
	InternalError        Type = "INTERNAL_ERROR"
)

// Sentinelas por categoría para reconocer errores mediante errors.Is, incluso
// después de envolverlos con %w.
var (
	ErrBadRequest           = &Error{Type: BadRequest}
	ErrValidation           = &Error{Type: ValidationError}
	ErrNotFound             = &Error{Type: NotFound}
	ErrConflict             = &Error{Type: Conflict}
	ErrPayloadTooLarge      = &Error{Type: PayloadTooLarge}
	ErrUnsupportedMediaType = &Error{Type: UnsupportedMediaType}
	ErrServiceUnavailable   = &Error{Type: ServiceUnavailable}
	ErrInternal             = &Error{Type: InternalError}
)

// Error conserva la clasificación estable, un código legible por clientes y
// un mensaje público. cause permanece privada y solo sirve para diagnóstico.
type Error struct {
	Type    Type
	Code    string
	Message string
	Details map[string]any
	cause   error
}

// New crea un error tipado sin causa interna.
func New(errorType Type, code, message string) *Error {
	return &Error{Type: errorType, Code: code, Message: message}
}

// Wrap crea un error tipado que conserva la causa para errors.Is/As y para
// diagnóstico interno. La causa no forma parte del mensaje público.
func Wrap(cause error, errorType Type, code, message string) *Error {
	return &Error{
		Type:    errorType,
		Code:    code,
		Message: message,
		cause:   cause,
	}
}

func (e *Error) Error() string {
	if e.Code == "" {
		return string(e.Type)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Code)
}

// Unwrap expone la causa solo al código del servidor.
func (e *Error) Unwrap() error {
	return e.cause
}

// Is compara categoría y, si el destino define código, también el código.
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Type == other.Type && (other.Code == "" || e.Code == other.Code)
}
