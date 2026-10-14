package dto

import "time"

// GetOrderOutput representa un pedido recuperado para presentación.
type GetOrderOutput struct {
	ID         string
	CustomerID string
	Items      []GetOrderItemOutput
	TotalMinor int64
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// GetOrderItemOutput representa un artículo recuperado.
type GetOrderItemOutput struct {
	ProductID      string
	Quantity       int64
	UnitPriceMinor int64
}
