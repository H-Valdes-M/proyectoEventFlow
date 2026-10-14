package entity

import (
	"math"

	"eventflow/pkg/apperrors"
)

// CalculateTotal suma los importes de articulos previamente validados. Cada
// UnitPriceMinor representa un peso CLP y el total se conserva como int64.
func CalculateTotal(items []OrderItem) (int64, error) {
	var total int64
	for _, item := range items {
		if item.Quantity > math.MaxInt64/item.UnitPriceMinor {
			return 0, totalOverflowError()
		}
		lineTotal := item.Quantity * item.UnitPriceMinor
		if total > math.MaxInt64-lineTotal {
			return 0, totalOverflowError()
		}
		total += lineTotal
	}
	return total, nil
}

func totalOverflowError() *apperrors.Error {
	err := apperrors.New(apperrors.ValidationError, "ORDER_TOTAL_OVERFLOW", "El total del pedido excede el máximo permitido")
	err.Details = map[string]any{"field": "items"}
	return err
}
