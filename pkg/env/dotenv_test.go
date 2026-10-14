package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDotEnvSinArchivo(t *testing.T) {
	values, err := LoadDotEnv(filepath.Join(t.TempDir(), ".env.dev"))
	if err != nil {
		t.Fatalf("la ausencia del archivo no debe ser un error: %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("sin archivo se esperaba un mapa vacio y se obtuvo %v", values)
	}
}

func TestLoadDotEnvLeePares(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.dev")
	content := "# comentario\r\n\r\nDB_NAME=eventflow\r\nDB_PORT=5432\nLANG=\"es-CL\"\nLOG='debug'\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("crear archivo de prueba: %v", err)
	}
	values, err := LoadDotEnv(path)
	if err != nil {
		t.Fatalf("la carga no debio fallar: %v", err)
	}
	expected := map[string]string{"DB_NAME": "eventflow", "DB_PORT": "5432", "LANG": "es-CL", "LOG": "debug"}
	if len(values) != len(expected) {
		t.Fatalf("se esperaban %d pares y hay %d: %v", len(expected), len(values), values)
	}
	for key, want := range expected {
		if values[key] != want {
			t.Fatalf("%s = %q, se esperaba %q", key, values[key], want)
		}
	}
}

func TestLoadDotEnvRechazaLineaMalformadaSinFiltrar(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.dev")
	content := "DB_NAME=eventflow\nDB_PASSWORD s3cr3to\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("crear archivo de prueba: %v", err)
	}
	_, err := LoadDotEnv(path)
	if err == nil {
		t.Fatal("una linea sin el separador = debe producir error")
	}
	if strings.Contains(err.Error(), "s3cr3to") {
		t.Fatalf("el error no debe reproducir el valor de la linea: %v", err)
	}
}

func TestLoadDotEnvRechazaClaveVacia(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.dev")
	if err := os.WriteFile(path, []byte("=valor\n"), 0o600); err != nil {
		t.Fatalf("crear archivo de prueba: %v", err)
	}
	if _, err := LoadDotEnv(path); err == nil {
		t.Fatal("una linea con clave vacia debe producir error")
	}
}
