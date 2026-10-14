package entity

import (
	"strings"
	"unicode/utf8"

	"eventflow/pkg/apperrors"
)

const (
	maxIdentifierRunes = 100
	maxItemQuantity    = int64(1_000_000)
)

// OrderItem representa una linea de pedido. UnitPriceMinor equivale a pesos
// CLP en este laboratorio.
type OrderItem struct {
	ProductID      string
	Quantity       int64
	UnitPriceMinor int64
}

// ValidateItem valida las invariantes de una linea y devuelve una copia con
// ProductID normalizado mediante TrimSpace.
func ValidateItem(item OrderItem) (OrderItem, error) {
	item.ProductID = strings.TrimSpace(item.ProductID)
	if length := utf8.RuneCountInString(item.ProductID); length == 0 || length > maxIdentifierRunes {
		return OrderItem{}, validationError("product_id", "El identificador del producto debe tener entre 1 y 100 caracteres")
	}
	if item.Quantity < 1 || item.Quantity > maxItemQuantity {
		return OrderItem{}, validationError("quantity", "La cantidad debe estar entre 1 y 1000000")
	}
	if item.UnitPriceMinor <= 0 {
		return OrderItem{}, validationError("unit_price_minor", "El precio unitario debe ser positivo")
	}
	return item, nil
}

func validationError(field, message string) *apperrors.Error {
	err := apperrors.New(apperrors.ValidationError, "ORDER_ITEM_VALIDATION", message)
	err.Details = map[string]any{"field": field}
	return err
}
