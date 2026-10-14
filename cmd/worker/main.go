package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"eventflow/cmd/app"
	"go.uber.org/zap"
)

func main() {
	os.Exit(run())
}

func run() int {
	resources, err := app.InitWorker()
	if err != nil {
		fmt.Fprintf(os.Stderr, "eventflow: inicializar worker: %v\n", err)
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- resources.Run(ctx) }()
	shutdownCompleted := false
	select {
	case err = <-runDone:
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), resources.APIConfig.ShutdownTimeout)
		defer shutdownCancel()
		if shutdownErr := resources.Shutdown(shutdownCtx); shutdownErr != nil {
			fmt.Fprintf(os.Stderr, "eventflow: cerrar worker: %v\n", shutdownErr)
			return 1
		}
		err = <-runDone
		shutdownCompleted = true
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		resources.Logger.Error("worker finalizo", zap.Error(err))
	}
	if !shutdownCompleted {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), resources.APIConfig.ShutdownTimeout)
		defer shutdownCancel()
		if shutdownErr := resources.Shutdown(shutdownCtx); shutdownErr != nil {
			fmt.Fprintf(os.Stderr, "eventflow: cerrar worker: %v\n", shutdownErr)
			return 1
		}
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		return 1
	}
	return 0
}
