package event

import (
	"testing"
	"time"
)

func TestEnvelopeSerializaYDeserializaCamposObligatorios(t *testing.T) {
	occurredAt := time.Date(2026, 10, 13, 20, 0, 0, 0, time.FixedZone("UTC-3", -3*60*60))
	original := NewOrderCreated("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333", occurredAt)

	payload, err := original.Serialize()
	if err != nil {
		t.Fatal("serializar envelope: ", err)
	}
	decoded, err := Deserialize(payload)
	if err != nil {
		t.Fatal("deserializar envelope: ", err)
	}
	if decoded != original {
		t.Fatalf("envelope distinto despues de serializar: got=%+v want=%+v", decoded, original)
	}
}
