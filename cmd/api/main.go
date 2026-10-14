package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"eventflow/cmd/app"
)

// @title EventFlow API
// @version 1.0
// @description API de pedidos EventFlow.
// @BasePath /
func main() {
	os.Exit(run())
}

func run() int {
	resources, err := app.Init()
	if err != nil {
		log.Fatalf("eventflow: inicializar API: %v", err)
		return 1
	}

	serverErrors := make(chan error, 1)
	go func() { serverErrors <- resources.Run() }()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	var exitCode int
	select {
	case err := <-serverErrors:
		if err != nil {
			log.Fatalf("eventflow: servidor HTTP finalizo: %v", err)
			exitCode = 1
		}
	case <-signals:
		log.Printf("eventflow: señal de cierre recibida")
	}

	ctx, cancel := context.WithTimeout(context.Background(), resources.Config.ShutdownTimeout)
	defer cancel()
	if err := resources.Shutdown(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("eventflow: cerrar API: %v", err)
		return 1
	}
	return exitCode
}
