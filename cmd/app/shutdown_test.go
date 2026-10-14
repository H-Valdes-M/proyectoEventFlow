package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"eventflow/cmd/routes"
	"github.com/gin-gonic/gin"
)

func TestShutdownMarcaNoListoAntesDeDrenarYEsperaSolicitudEnCurso(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	completed := make(chan struct{})
	readiness := newReadinessGate(readinessDouble{})
	handler := routes.New(nil, routes.WithReadiness(readiness))
	handler.GET("/work", func(c *gin.Context) {
		close(started)
		select {
		case <-release:
			c.Status(http.StatusNoContent)
			close(completed)
		case <-c.Request.Context().Done():
		}
	})

	server := httptest.NewUnstartedServer(handler)
	server.Start()
	defer server.Close()
	database := &databaseDouble{}
	resources := &Resources{Server: server.Config, Database: database, readiness: readiness}

	requestDone := make(chan error, 1)
	go func() {
		response, err := server.Client().Get(server.URL + "/work")
		if err == nil {
			response.Body.Close()
		}
		requestDone <- err
	}()
	<-started

	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		shutdownDone <- resources.Shutdown(ctx)
	}()

	for readiness.Execute(context.Background()) == nil {
		select {
		case err := <-shutdownDone:
			t.Fatalf("shutdown finalizo antes de drenar: %v", err)
		default:
		}
	}
	if !errors.Is(readiness.Execute(context.Background()), errAPIShuttingDown) {
		t.Fatal("readiness debe fallar durante el drenaje")
	}
	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz durante drenaje status=%d", ready.Code)
	}
	if database.closed {
		t.Fatal("la base no debe cerrarse mientras la solicitud sigue en curso")
	}

	close(release)
	<-completed
	if err := <-requestDone; err != nil {
		t.Fatalf("solicitud drenada: %v", err)
	}
	if err := <-shutdownDone; err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if !database.closed {
		t.Fatal("la base debe cerrarse al terminar el drenaje")
	}
}

func TestShutdownVencidoCancelaSolicitudActiva(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	readiness := newReadinessGate(readinessDouble{})
	handler := routes.New(nil, routes.WithReadiness(readiness))
	handler.GET("/work", func(c *gin.Context) {
		close(started)
		<-c.Request.Context().Done()
		close(canceled)
	})

	server := httptest.NewUnstartedServer(handler)
	server.Start()
	defer server.Close()
	database := &databaseDouble{}
	resources := &Resources{Server: server.Config, Database: database, readiness: readiness}
	requestDone := make(chan error, 1)
	go func() {
		response, err := server.Client().Get(server.URL + "/work")
		if err == nil {
			response.Body.Close()
		}
		requestDone <- err
	}()
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := resources.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown vencido error=%v", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("la solicitud activa no fue cancelada")
	}
	if !database.closed {
		t.Fatal("la base debe cerrarse tras cancelar la solicitud")
	}
	if err := <-requestDone; err == nil {
		t.Fatal("la solicitud cancelada no debe completar normalmente")
	}
}

type readinessDouble struct{}

func (readinessDouble) Execute(context.Context) error { return nil }

type databaseDouble struct{ closed bool }

func (d *databaseDouble) Ping(context.Context) error { return nil }
func (d *databaseDouble) Close()                     { d.closed = true }
func (d *databaseDouble) Native() any                { return nil }
