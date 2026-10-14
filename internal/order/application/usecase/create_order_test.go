package usecase

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"eventflow/internal/order/application/dto"
	"eventflow/internal/order/domain/entity"
	"eventflow/internal/order/domain/repository"
	"eventflow/pkg/apperrors"
)

func TestCreateOrderGuardaDatosValidosUnaVez(t *testing.T) {
	repo := &repositoryDouble{}
	ctx := context.WithValue(context.Background(), contextKey("request"), "value")
	useCase := NewCreateOrderUseCase(repo, fixedNow, fixedID)
	output, err := useCase.Execute(ctx, validInput())
	if err != nil {
		t.Fatalf("ejecutar caso de uso: %v", err)
	}
	if repo.saves != 1 || repo.context != ctx || repo.order.CustomerID != "cliente-001" || output.ID != fixedOrderID || output.Status != "pending" || output.TotalMinor != 3000 {
		t.Fatalf("resultado inesperado: output=%+v repo=%+v", output, repo)
	}
}

func TestCreateOrderNoGuardaDatosInvalidos(t *testing.T) {
	repo := &repositoryDouble{}
	useCase := NewCreateOrderUseCase(repo, fixedNow, fixedID)
	_, err := useCase.Execute(context.Background(), dto.CreateOrderInput{CustomerID: "", Items: validInput().Items})
	if !errors.Is(err, apperrors.ErrValidation) || repo.saves != 0 {
		t.Fatalf("los datos invalidos no deben guardarse: err=%v saves=%d", err, repo.saves)
	}
}

func TestCreateOrderPropagaFalloDePersistencia(t *testing.T) {
	persistenceFailure := apperrors.Wrap(errors.New("conexion perdida"), apperrors.InternalError, "ORDER_PERSISTENCE", "No se pudo guardar")
	repo := &repositoryDouble{err: persistenceFailure}
	useCase := NewCreateOrderUseCase(repo, fixedNow, fixedID)
	_, err := useCase.Execute(context.Background(), validInput())
	if !errors.Is(err, apperrors.ErrInternal) || !errors.Is(err, persistenceFailure) {
		t.Fatalf("el fallo debe conservar clasificacion y causa: %v", err)
	}
}

func TestNewUUIDGeneraFormatoV4(t *testing.T) {
	id, err := NewUUID()
	if err != nil || !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(id) {
		t.Fatalf("UUID inesperado: id=%q err=%v", id, err)
	}
}

const fixedOrderID = "77777777-7777-4777-8777-777777777777"

func fixedID() (string, error) { return fixedOrderID, nil }

func fixedNow() time.Time { return time.Date(2026, 10, 13, 19, 0, 0, 0, time.UTC) }

func validInput() dto.CreateOrderInput {
	return dto.CreateOrderInput{CustomerID: " cliente-001 ", Items: []dto.CreateOrderItemInput{{ProductID: "producto-001", Quantity: 2, UnitPriceMinor: 1500}}}
}

type contextKey string

type repositoryDouble struct {
	context context.Context
	order   entity.Order
	saves   int
	err     error
}

func (d *repositoryDouble) Save(ctx context.Context, order entity.Order) error {
	d.context = ctx
	d.order = order
	d.saves++
	return d.err
}

func (d *repositoryDouble) FindByID(context.Context, string) (entity.Order, error) {
	return entity.Order{}, repository.ErrOrderNotFound
}
