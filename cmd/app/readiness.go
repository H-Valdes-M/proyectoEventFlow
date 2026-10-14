package app

import (
	"context"
	"errors"
	"sync/atomic"
)

var errAPIShuttingDown = errors.New("api en apagado")

type readinessExecutor interface {
	Execute(context.Context) error
}

// readinessGate evita declarar lista la API una vez iniciado su apagado.
type readinessGate struct {
	accepting atomic.Bool
	delegate  readinessExecutor
}

func newReadinessGate(delegate readinessExecutor) *readinessGate {
	gate := &readinessGate{delegate: delegate}
	gate.accepting.Store(true)
	return gate
}

func (g *readinessGate) Execute(ctx context.Context) error {
	if !g.accepting.Load() {
		return errAPIShuttingDown
	}
	return g.delegate.Execute(ctx)
}

func (g *readinessGate) markNotReady() {
	if g != nil {
		g.accepting.Store(false)
	}
}
