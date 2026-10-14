package usecase

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"eventflow/internal/order/application/dto"
	"eventflow/internal/order/domain/entity"
)

const fingerprintContractVersion = 1

type canonicalCreateOrder struct {
	ContractVersion int                  `json:"contract_version"`
	Operation       string               `json:"operation"`
	CustomerID      string               `json:"customer_id"`
	Items           []canonicalOrderItem `json:"items"`
}

type canonicalOrderItem struct {
	ProductID      string `json:"product_id"`
	Quantity       int64  `json:"quantity"`
	UnitPriceMinor int64  `json:"unit_price_minor"`
}

// CanonicalFingerprint calcula una huella estable de una creación válida.
func CanonicalFingerprint(input dto.CreateOrderInput) (string, error) {
	items := make([]entity.OrderItem, len(input.Items))
	for index, item := range input.Items {
		items[index] = entity.OrderItem{ProductID: item.ProductID, Quantity: item.Quantity, UnitPriceMinor: item.UnitPriceMinor}
	}
	order, err := entity.NewOrder("00000000-0000-4000-8000-000000000000", input.CustomerID, items, time.Unix(0, 0).UTC())
	if err != nil {
		return "", err
	}
	canonical := canonicalCreateOrder{ContractVersion: fingerprintContractVersion, Operation: "POST /orders", CustomerID: order.CustomerID, Items: make([]canonicalOrderItem, len(order.Items))}
	for index, item := range order.Items {
		canonical.Items[index] = canonicalOrderItem{ProductID: item.ProductID, Quantity: item.Quantity, UnitPriceMinor: item.UnitPriceMinor}
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
