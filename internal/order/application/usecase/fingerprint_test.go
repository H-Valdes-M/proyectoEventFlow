package usecase

import (
	"testing"

	"eventflow/internal/order/application/dto"
)

func TestCanonicalFingerprintNormalizaEspaciosYOrden(t *testing.T) {
	first := dto.CreateOrderInput{CustomerID: " cliente ", Items: []dto.CreateOrderItemInput{{ProductID: " b ", Quantity: 2, UnitPriceMinor: 1500}, {ProductID: "a", Quantity: 1, UnitPriceMinor: 200}}}
	second := dto.CreateOrderInput{CustomerID: "cliente", Items: []dto.CreateOrderItemInput{{ProductID: " a ", Quantity: 1, UnitPriceMinor: 200}, {ProductID: "b", Quantity: 2, UnitPriceMinor: 1500}}}
	firstFingerprint, err := CanonicalFingerprint(first)
	if err != nil {
		t.Fatal(err)
	}
	secondFingerprint, err := CanonicalFingerprint(second)
	if err != nil || firstFingerprint != secondFingerprint {
		t.Fatalf("huellas equivalentes inesperadas: first=%q second=%q err=%v", firstFingerprint, secondFingerprint, err)
	}
}

func TestCanonicalFingerprintCambiaConDatosDeNegocio(t *testing.T) {
	base := dto.CreateOrderInput{CustomerID: "cliente", Items: []dto.CreateOrderItemInput{{ProductID: "producto", Quantity: 2, UnitPriceMinor: 1500}}}
	baseFingerprint, err := CanonicalFingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	changes := []dto.CreateOrderInput{
		{CustomerID: "otro-cliente", Items: base.Items},
		{CustomerID: base.CustomerID, Items: []dto.CreateOrderItemInput{{ProductID: "producto", Quantity: 3, UnitPriceMinor: 1500}}},
		{CustomerID: base.CustomerID, Items: []dto.CreateOrderItemInput{{ProductID: "producto", Quantity: 2, UnitPriceMinor: 1600}}},
	}
	for _, changed := range changes {
		fingerprint, err := CanonicalFingerprint(changed)
		if err != nil || fingerprint == baseFingerprint {
			t.Fatalf("cambio no reflejado: fingerprint=%q err=%v", fingerprint, err)
		}
	}
}
