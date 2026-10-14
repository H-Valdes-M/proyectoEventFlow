package apperrors

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorConservaTipoYCodigoAlEnvolver(t *testing.T) {
	cause := errors.New("violacion de unique con datos internos")
	typed := Wrap(cause, Conflict, "IDEMPOTENCY_CONFLICT", "La clave ya fue usada con otros datos")
	wrapped := fmt.Errorf("crear pedido: %w", typed)

	if !errors.Is(wrapped, ErrConflict) {
		t.Fatal("errors.Is debe reconocer la categoria despues de envolver")
	}
	if !errors.Is(wrapped, cause) {
		t.Fatal("errors.Is debe conservar la causa privada")
	}

	var extracted *Error
	if !errors.As(wrapped, &extracted) {
		t.Fatal("errors.As debe recuperar el error tipado")
	}
	if extracted.Code != "IDEMPOTENCY_CONFLICT" || extracted.Message != "La clave ya fue usada con otros datos" {
		t.Fatalf("error recuperado inesperado: %+v", extracted)
	}
}

func TestErrorIsPuedeCompararCodigo(t *testing.T) {
	err := New(ValidationError, "ORDER_VALIDATION", "Pedido invalido")
	if !errors.Is(err, &Error{Type: ValidationError, Code: "ORDER_VALIDATION"}) {
		t.Fatal("errors.Is debe reconocer tipo y codigo iguales")
	}
	if errors.Is(err, &Error{Type: ValidationError, Code: "OTHER"}) {
		t.Fatal("errors.Is no debe reconocer un codigo diferente")
	}
}
