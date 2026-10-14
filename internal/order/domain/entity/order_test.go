package entity

import (
	"errors"
	"strings"
	"testing"
	"time"

	"eventflow/pkg/apperrors"
)

func TestNewOrderConstruyePedidoNormalizadoYCanonico(t *testing.T) {
	now := time.Date(2026, 10, 13, 15, 0, 0, 0, time.FixedZone("UTC-3", -3*60*60))
	order, err := NewOrder("order-001", "  cliente-001  ", []OrderItem{
		{ProductID: " producto-b ", Quantity: 2, UnitPriceMinor: 1500},
		{ProductID: "producto-a", Quantity: 3, UnitPriceMinor: 200},
	}, now)
	if err != nil {
		t.Fatalf("construir pedido: %v", err)
	}
	if order.ID != "order-001" || order.CustomerID != "cliente-001" || order.TotalMinor != 3600 || order.Status != OrderPending {
		t.Fatalf("pedido inesperado: %+v", order)
	}
	if order.Items[0].ProductID != "producto-a" || order.Items[1].ProductID != "producto-b" {
		t.Fatalf("articulos no quedaron en orden canonico: %+v", order.Items)
	}
	if order.CreatedAt.Location() != time.UTC || !order.CreatedAt.Equal(now.UTC()) || !order.UpdatedAt.Equal(now.UTC()) {
		t.Fatalf("timestamps inesperados: created=%s updated=%s", order.CreatedAt, order.UpdatedAt)
	}
}

func TestNewOrderRechazaClienteOListaInvalidos(t *testing.T) {
	cases := []struct {
		name       string
		customerID string
		items      []OrderItem
		field      string
	}{
		{name: "cliente vacio", customerID: "  ", items: []OrderItem{validItem("producto-001")}, field: "customer_id"},
		{name: "cliente de 101 runas", customerID: strings.Repeat("ñ", 101), items: []OrderItem{validItem("producto-001")}, field: "customer_id"},
		{name: "sin articulos", customerID: "cliente-001", items: nil, field: "items"},
		{name: "101 articulos", customerID: "cliente-001", items: repeatedItems(101), field: "items"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			order, err := NewOrder("order-001", tc.customerID, tc.items, time.Now())
			assertOrderValidation(t, err, tc.field)
			assertZeroOrder(t, order)
		})
	}
}

func TestNewOrderRechazaProductoDuplicadoTrasNormalizar(t *testing.T) {
	order, err := NewOrder("order-001", "cliente-001", []OrderItem{
		validItem("producto-001"),
		validItem(" producto-001 "),
	}, time.Now())
	assertOrderValidation(t, err, "items")
	assertZeroOrder(t, order)
}

func TestNewOrderConservaSensibilidadAMayusculas(t *testing.T) {
	order, err := NewOrder("order-001", "cliente-001", []OrderItem{
		validItem("Producto-001"),
		validItem("producto-001"),
	}, time.Now())
	if err != nil || len(order.Items) != 2 {
		t.Fatalf("los identificadores con distinta mayuscula son distintos: order=%+v err=%v", order, err)
	}
}

func repeatedItems(count int) []OrderItem {
	items := make([]OrderItem, count)
	for index := range items {
		items[index] = validItem("producto-001")
	}
	return items
}

func assertOrderValidation(t *testing.T, err error, field string) {
	t.Helper()
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("se esperaba error de validacion, llego %v", err)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Details["field"] != field {
		t.Fatalf("details inesperados: %+v", typed)
	}
}

func assertZeroOrder(t *testing.T, order Order) {
	t.Helper()
	if order.ID != "" || order.CustomerID != "" || len(order.Items) != 0 || order.TotalMinor != 0 || order.Status != "" || !order.CreatedAt.IsZero() || !order.UpdatedAt.IsZero() {
		t.Fatalf("no se debe devolver pedido parcial: %+v", order)
	}
}
