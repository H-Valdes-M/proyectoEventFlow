package entity

import (
	"errors"
	"math"
	"testing"

	"eventflow/pkg/apperrors"
)

func TestCalculateTotalCalculaUnaLineaYVariasLineas(t *testing.T) {
	tests := []struct {
		name  string
		items []OrderItem
		want  int64
	}{
		{
			name:  "dos unidades a 1500",
			items: []OrderItem{{ProductID: "producto-001", Quantity: 2, UnitPriceMinor: 1500}},
			want:  3000,
		},
		{
			name: "varias lineas",
			items: []OrderItem{
				{ProductID: "producto-001", Quantity: 2, UnitPriceMinor: 1500},
				{ProductID: "producto-002", Quantity: 3, UnitPriceMinor: 200},
			},
			want: 3600,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, err := CalculateTotal(tt.items)
			if err != nil {
				t.Fatalf("calcular total: %v", err)
			}
			if total != tt.want {
				t.Fatalf("total = %d, se esperaba %d", total, tt.want)
			}
		})
	}
}

func TestCalculateTotalRechazaOverflowDeProducto(t *testing.T) {
	_, err := CalculateTotal([]OrderItem{{Quantity: 2, UnitPriceMinor: math.MaxInt64}})
	assertTotalOverflow(t, err)
}

func TestCalculateTotalRechazaOverflowDeAcumulacion(t *testing.T) {
	lineTotal := int64(math.MaxInt64/2 + 1)
	_, err := CalculateTotal([]OrderItem{
		{Quantity: 1, UnitPriceMinor: lineTotal},
		{Quantity: 1, UnitPriceMinor: lineTotal},
	})
	assertTotalOverflow(t, err)
}

func assertTotalOverflow(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("se esperaba error de validacion, llego %v", err)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Code != "ORDER_TOTAL_OVERFLOW" {
		t.Fatalf("error de overflow inesperado: %+v", typed)
	}
}
