package dto

// CreateOrderInput contiene los datos para crear un pedido.
type CreateOrderInput struct {
	CustomerID string
	Items      []CreateOrderItemInput
}

// CreateOrderItemInput representa un artículo recibido por la aplicación.
type CreateOrderItemInput struct {
	ProductID      string
	Quantity       int64
	UnitPriceMinor int64
}

// CreateOrderOutput resume el pedido creado para capas de presentación futuras.
type CreateOrderOutput struct {
	ID         string
	Status     string
	TotalMinor int64
}
