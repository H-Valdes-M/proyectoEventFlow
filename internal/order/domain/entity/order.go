package entity

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"eventflow/pkg/apperrors"
)

const maxOrderItems = 100

// OrderStatus representa los estados de negocio que puede tener un pedido.
type OrderStatus string

const OrderPending OrderStatus = "pending"

// Order representa un pedido validado y listo para que capas posteriores lo
// persistan. TotalMinor se expresa en pesos CLP.
type Order struct {
	ID         string
	CustomerID string
	Items      []OrderItem
	TotalMinor int64
	Status     OrderStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewOrder construye un pedido completo sin efectos externos. El identificador
// y el instante se reciben para que el constructor sea determinista en pruebas.
func NewOrder(id, customerID string, items []OrderItem, now time.Time) (Order, error) {
	customerID = strings.TrimSpace(customerID)
	if length := utf8.RuneCountInString(customerID); length == 0 || length > maxIdentifierRunes {
		return Order{}, orderValidationError("customer_id", "El identificador del cliente debe tener entre 1 y 100 caracteres")
	}
	if len(items) == 0 || len(items) > maxOrderItems {
		return Order{}, orderValidationError("items", "El pedido debe tener entre 1 y 100 articulos")
	}

	normalizedItems := make([]OrderItem, 0, len(items))
	productIDs := make(map[string]struct{}, len(items))
	for _, item := range items {
		normalized, err := ValidateItem(item)
		if err != nil {
			return Order{}, err
		}
		if _, exists := productIDs[normalized.ProductID]; exists {
			return Order{}, orderValidationError("items", "Los productos del pedido no se pueden repetir")
		}
		productIDs[normalized.ProductID] = struct{}{}
		normalizedItems = append(normalizedItems, normalized)
	}
	sort.Slice(normalizedItems, func(i, j int) bool {
		return normalizedItems[i].ProductID < normalizedItems[j].ProductID
	})

	total, err := CalculateTotal(normalizedItems)
	if err != nil {
		return Order{}, err
	}
	now = now.UTC()
	return Order{
		ID:         id,
		CustomerID: customerID,
		Items:      normalizedItems,
		TotalMinor: total,
		Status:     OrderPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func orderValidationError(field, message string) *apperrors.Error {
	err := apperrors.New(apperrors.ValidationError, "ORDER_VALIDATION", message)
	err.Details = map[string]any{"field": field}
	return err
}
