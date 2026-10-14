package application

import "context"

// ReadinessChecker comprueba las dependencias necesarias para atender pedidos.
type ReadinessChecker interface {
	Ready(context.Context) error
}

// ReadinessUseCase coordina la comprobación de disponibilidad de la API.
type ReadinessUseCase struct {
	checker ReadinessChecker
}

// NewReadinessUseCase construye el caso de uso de readiness.
func NewReadinessUseCase(checker ReadinessChecker) *ReadinessUseCase {
	return &ReadinessUseCase{checker: checker}
}

// Execute devuelve el error de dependencia sin mezclar transporte HTTP.
func (u *ReadinessUseCase) Execute(ctx context.Context) error {
	return u.checker.Ready(ctx)
}
