package app

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
)

func TestInitRegistraSoloCamposSeguros(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ENVIRONMENT", "dev")
	t.Setenv("DB_PASSWORD", "clave-super-secreta")
	var output bytes.Buffer

	resources, err := initWithWriter(&output)
	if err != nil {
		t.Fatalf("inicializar recursos: %v", err)
	}
	if err := resources.Shutdown(context.Background()); err != nil {
		t.Fatalf("cerrar logger: %v", err)
	}

	log := output.String()
	if !strings.Contains(log, `"environment":"dev"`) {
		t.Fatalf("el log de arranque debe contener el entorno: %q", log)
	}
	if strings.Contains(log, "clave-super-secreta") || strings.Contains(log, "postgres://") {
		t.Fatalf("el log no debe filtrar contrasena ni DSN: %q", log)
	}
}

func TestRunDevuelveErrorSiElPuertoEstaOcupado(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reservar puerto: %v", err)
	}
	defer listener.Close()

	resources := &Resources{Server: newTestServer(listener.Addr().String())}
	if err := resources.Run(); err == nil {
		t.Fatal("un puerto ocupado debe devolver un error de arranque")
	}
}

func TestInitFallaConConfiguracionInvalida(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ENVIRONMENT", "staging")
	if _, err := initWithWriter(&bytes.Buffer{}); err == nil {
		t.Fatal("la configuracion invalida debe impedir inicializar la aplicacion")
	}
}
