package env

import "testing"

func TestNormalizeDevPorDefecto(t *testing.T) {
	if Normalize("") != "dev" {
		t.Fatal("un valor vacio debe normalizarse a dev")
	}
	if Normalize("   ") != "dev" {
		t.Fatal("un valor de espacios se considera vacio y debe normalizarse a dev")
	}
}

func TestNormalizeRecortaYConservaValores(t *testing.T) {
	cases := map[string]string{
		"dev":   "dev",
		" qa ":  "qa",
		"prod":  "prod",
		"\tdev": "dev",
	}
	for input, expected := range cases {
		if got := Normalize(input); got != expected {
			t.Fatalf("Normalize(%q) = %q, se esperaba %q", input, got, expected)
		}
	}
}
