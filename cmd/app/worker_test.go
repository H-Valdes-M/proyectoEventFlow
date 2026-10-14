package app

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"eventflow/cmd/config"
)

func TestRunRelayLoopEjecutaInmediatamenteYRespetaCancelacion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	executed := make(chan struct{}, 1)
	finished := make(chan error, 1)
	go func() {
		finished <- runRelayLoop(ctx, time.Hour, func(context.Context) error {
			executed <- struct{}{}
			return errors.New("broker no disponible")
		})
	}()
	select {
	case <-executed:
	case <-time.After(time.Second):
		t.Fatal("el primer lote debe ejecutarse sin esperar el intervalo")
	}
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("el cierre cancelado no debe devolver error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("el ciclo no respondió a la cancelación")
	}
}

func TestWorkerReadyExigeBrokerYBase(t *testing.T) {
	resources := &WorkerResources{Database: &workerDatabaseDouble{}}
	if err := resources.Ready(context.Background()); err == nil {
		t.Fatal("un worker sin broker no puede estar listo")
	}
	resources.setReady(true)
	if err := resources.Ready(context.Background()); err != nil {
		t.Fatalf("dependencias disponibles deben dejar listo al worker: %v", err)
	}
}

func TestWorkerShutdownDrenaEntregaActivaAntesDeCerrarRecursos(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	committed := make(chan struct{})
	database := &workerDatabaseDouble{}
	resources := &WorkerResources{
		Config:   &config.WorkerConfig{},
		Database: database,
		runWorker: func(_ context.Context, workCtx context.Context) error {
			close(started)
			select {
			case <-release:
				close(committed)
				return nil
			case <-workCtx.Done():
				return workCtx.Err()
			}
		},
	}
	resources.setReady(true)
	runCtx, stopRun := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- resources.Run(runCtx) }()
	<-started

	stopRun() // Detiene la entrada, pero no cancela la entrega ya seleccionada.
	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		shutdownDone <- resources.Shutdown(ctx)
	}()

	for resources.Ready(context.Background()) == nil {
		runtime.Gosched()
	}
	if database.closed {
		t.Fatal("la base no debe cerrarse mientras la entrega esta activa")
	}
	close(release)
	select {
	case <-committed:
	case <-time.After(time.Second):
		t.Fatal("la entrega activa no termino dentro del drenaje")
	}
	if err := <-shutdownDone; err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if err := <-runDone; err != nil {
		t.Fatalf("run: %v", err)
	}
	if !database.closed {
		t.Fatal("la base debe cerrarse despues del drenaje")
	}
}

func TestWorkerShutdownVencidoCancelaEntregaParaRollbackRecuperable(t *testing.T) {
	started := make(chan struct{})
	rolledBack := make(chan struct{})
	database := &workerDatabaseDouble{}
	resources := &WorkerResources{
		Config:   &config.WorkerConfig{},
		Database: database,
		runWorker: func(_ context.Context, workCtx context.Context) error {
			close(started)
			<-workCtx.Done()
			close(rolledBack)
			return workCtx.Err()
		},
	}
	runCtx, stopRun := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- resources.Run(runCtx) }()
	<-started
	stopRun()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := resources.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown vencido error=%v", err)
	}
	select {
	case <-rolledBack:
	case <-time.After(time.Second):
		t.Fatal("la entrega activa no recibio cancelacion para rollback")
	}
	if err := <-runDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("run tras cancelacion=%v", err)
	}
	if !database.closed {
		t.Fatal("la base debe cerrarse despues de cancelar la entrega")
	}
}

type workerDatabaseDouble struct{ closed bool }

func (workerDatabaseDouble) Ping(context.Context) error { return nil }
func (d *workerDatabaseDouble) Close()                  { d.closed = true }
func (workerDatabaseDouble) Native() any                { return nil }
