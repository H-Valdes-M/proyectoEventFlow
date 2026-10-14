package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewWithWriterFiltraPorNivelYEmiteJSON(t *testing.T) {
	var output bytes.Buffer
	log, err := NewWithWriter("warn", "json", &output)
	if err != nil {
		t.Fatalf("crear logger: %v", err)
	}

	log.Info("mensaje ignorado")
	log.Error("mensaje visible", zap.String("operation", "prueba"))
	if err := log.Sync(); err != nil {
		t.Fatalf("sincronizar logger: %v", err)
	}

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("la salida debe ser JSON legible: %v; salida=%q", err, output.String())
	}
	if entry["level"] != "error" || entry["msg"] != "mensaje visible" {
		t.Fatalf("entrada inesperada: %v", entry)
	}
	if _, err := time.Parse(time.RFC3339, entry["ts"].(string)); err != nil {
		t.Fatalf("timestamp no RFC3339: %v", err)
	}
	if strings.Contains(output.String(), "mensaje ignorado") {
		t.Fatalf("el nivel warn no debe escribir INFO: %q", output.String())
	}
}

func TestNewWithWriterConsole(t *testing.T) {
	var output bytes.Buffer
	log, err := NewWithWriter("info", "console", &output)
	if err != nil {
		t.Fatalf("crear logger console: %v", err)
	}
	log.Info("mensaje de consola")
	if err := log.Sync(); err != nil {
		t.Fatalf("sincronizar logger: %v", err)
	}
	if !strings.Contains(output.String(), "mensaje de consola") {
		t.Fatalf("la salida console debe incluir el mensaje: %q", output.String())
	}
	if json.Valid(output.Bytes()) {
		t.Fatalf("el formato console no debe serializarse como JSON: %q", output.String())
	}
}

func TestNewRechazaValoresInvalidos(t *testing.T) {
	if _, err := NewWithWriter("trace", "json", &bytes.Buffer{}); err == nil {
		t.Fatal("un nivel invalido debe fallar")
	}
	if _, err := NewWithWriter("info", "texto", &bytes.Buffer{}); err == nil {
		t.Fatal("un formato invalido debe fallar")
	}
}

func TestNewNopNoEscribe(t *testing.T) {
	log := NewNop()
	log.Info("mensaje descartado")
	if err := log.Sync(); err != nil {
		t.Fatalf("sincronizar logger nulo: %v", err)
	}
}
