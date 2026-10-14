package entity

import (
	"errors"
	"strings"
	"testing"

	"eventflow/pkg/apperrors"
)

func TestValidateItemNormalizaYAceptaArticuloValido(t *testing.T) {
	item, err := ValidateItem(OrderItem{
		ProductID:      "  producto-001  ",
		Quantity:       2,
		UnitPriceMinor: 1500,
	})
	if err != nil {
		t.Fatalf("validar articulo: %v", err)
	}
	if item.ProductID != "producto-001" {
		t.Fatalf("product_id normalizado = %q, se esperaba producto-001", item.ProductID)
	}
}

func TestValidateItemRechazaProductIDFueraDeLimitesUnicode(t *testing.T) {
	cases := []struct {
		name      string
		productID string
	}{
		{name: "vacio", productID: "   "},
		{name: "101 runas", productID: strings.Repeat("ñ", 101)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateItem(validItem(tc.productID))
			assertValidationField(t, err, "product_id")
		})
	}
}

func TestValidateItemAcepta100RunasUnicode(t *testing.T) {
	productID := strings.Repeat("ñ", 100)
	item, err := ValidateItem(validItem(productID))
	if err != nil {
		t.Fatalf("100 runas Unicode deben ser validas: %v", err)
	}
	if item.ProductID != productID {
		t.Fatalf("product_id inesperado: %q", item.ProductID)
	}
}

func TestValidateItemRechazaCantidadFueraDeRango(t *testing.T) {
	for _, quantity := range []int64{-1, 0, maxItemQuantity + 1} {
		t.Run("cantidad", func(t *testing.T) {
			item := validItem("producto-001")
			item.Quantity = quantity
			_, err := ValidateItem(item)
			assertValidationField(t, err, "quantity")
		})
	}
}

func TestValidateItemRechazaPrecioNoPositivo(t *testing.T) {
	for _, price := range []int64{-1, 0} {
		t.Run("precio", func(t *testing.T) {
			item := validItem("producto-001")
			item.UnitPriceMinor = price
			_, err := ValidateItem(item)
			assertValidationField(t, err, "unit_price_minor")
		})
	}
}

func validItem(productID string) OrderItem {
	return OrderItem{ProductID: productID, Quantity: 1, UnitPriceMinor: 1}
}

func assertValidationField(t *testing.T, err error, field string) {
	t.Helper()
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("se esperaba error de validacion, llego %v", err)
	}
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Details["field"] != field {
		t.Fatalf("details inesperados: %+v", typed)
	}
}
