package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"time"

	"eventflow/internal/order/application/dto"
	"eventflow/pkg/apperrors"
	"eventflow/pkg/httputil"
	"github.com/gin-gonic/gin"
)

const maxRequestBodyBytes = 1 << 20

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

// CreateOrderExecutor es el puerto del handler de creación.
type CreateOrderExecutor interface {
	Execute(context.Context, dto.CreateOrderInput, string) (dto.CreateOrderOutput, error)
}

// GetOrderExecutor es el puerto del handler de consulta.
type GetOrderExecutor interface {
	Execute(context.Context, string) (dto.GetOrderOutput, error)
}

// OrderHandler adapta HTTP a los casos de uso de pedidos.
type OrderHandler struct {
	create            CreateOrderExecutor
	get               GetOrderExecutor
	afterCreateCommit func(context.Context)
}

// NewOrderHandler construye el handler de pedidos.
func NewOrderHandler(create CreateOrderExecutor, get GetOrderExecutor) *OrderHandler {
	return &OrderHandler{create: create, get: get}
}

// Create godoc
// @Summary Crear pedido
// @Tags orders
// @Accept json
// @Produce json
// @Param Idempotency-Key header string true "Clave ASCII imprimible sin espacios (1-128)"
// @Param request body createOrderRequest true "Pedido"
// @Success 201 {object} createOrderResponse
// @Failure 400,409,413,415,422,500 {object} errorResponse
// @Router /orders [post]
func (h *OrderHandler) Create(c *gin.Context) {
	key, err := validateIdempotencyKey(c.GetHeader("Idempotency-Key"))
	if err != nil {
		abortWithError(c, err)
		return
	}
	input, err := decodeCreateRequest(c)
	if err != nil {
		abortWithError(c, err)
		return
	}
	output, err := h.create.Execute(c.Request.Context(), input, key)
	if err != nil {
		abortWithError(c, err)
		return
	}
	c.Set("order_id", output.ID)
	if h.afterCreateCommit != nil {
		h.afterCreateCommit(c.Request.Context())
	}
	c.Header("Location", "/orders/"+output.ID)
	c.JSON(http.StatusCreated, createOrderResponse{ID: output.ID, Status: output.Status, TotalMinor: output.TotalMinor})
}

func validateIdempotencyKey(key string) (string, error) {
	if len(key) < 1 || len(key) > 128 {
		return "", apperrors.New(apperrors.BadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key debe tener entre 1 y 128 caracteres")
	}
	for _, character := range []byte(key) {
		if character < 33 || character > 126 {
			return "", apperrors.New(apperrors.BadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key debe usar ASCII imprimible sin espacios")
		}
	}
	return key, nil
}

// GetByID godoc
// @Summary Consultar pedido
// @Tags orders
// @Produce json
// @Param id path string true "UUID del pedido"
// @Success 200 {object} getOrderResponse
// @Failure 400,404,500 {object} errorResponse
// @Router /orders/{id} [get]
func (h *OrderHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	if !uuidPattern.MatchString(id) {
		abortWithError(c, apperrors.New(apperrors.BadRequest, "INVALID_ORDER_ID", "El identificador del pedido no es valido"))
		return
	}
	output, err := h.get.Execute(c.Request.Context(), id)
	if err != nil {
		abortWithError(c, err)
		return
	}
	c.Set("order_id", output.ID)
	items := make([]orderItemResponse, len(output.Items))
	for index, item := range output.Items {
		items[index] = orderItemResponse{ProductID: item.ProductID, Quantity: item.Quantity, UnitPriceMinor: item.UnitPriceMinor}
	}
	c.JSON(http.StatusOK, getOrderResponse{ID: output.ID, CustomerID: output.CustomerID, Items: items, TotalMinor: output.TotalMinor, Status: output.Status, CreatedAt: output.CreatedAt.UTC(), UpdatedAt: output.UpdatedAt.UTC()})
}

type createOrderRequest struct {
	CustomerID string             `json:"customer_id"`
	Items      []orderItemRequest `json:"items"`
}

type orderItemRequest struct {
	ProductID      string `json:"product_id"`
	Quantity       int64  `json:"quantity"`
	UnitPriceMinor int64  `json:"unit_price_minor"`
}

type createOrderResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	TotalMinor int64  `json:"total_minor"`
}

type getOrderResponse struct {
	ID         string              `json:"id"`
	CustomerID string              `json:"customer_id"`
	Items      []orderItemResponse `json:"items"`
	TotalMinor int64               `json:"total_minor"`
	Status     string              `json:"status"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

type orderItemResponse struct {
	ProductID      string `json:"product_id"`
	Quantity       int64  `json:"quantity"`
	UnitPriceMinor int64  `json:"unit_price_minor"`
}

type errorResponse httputil.ErrorResponse

func decodeCreateRequest(c *gin.Context) (dto.CreateOrderInput, error) {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return dto.CreateOrderInput{}, apperrors.New(apperrors.UnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "El contenido debe ser application/json")
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var request createOrderRequest
	if err := decoder.Decode(&request); err != nil {
		return dto.CreateOrderInput{}, decodeError(err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return dto.CreateOrderInput{}, apperrors.New(apperrors.BadRequest, "INVALID_JSON", "El cuerpo debe contener un solo objeto JSON")
	}
	items := make([]dto.CreateOrderItemInput, len(request.Items))
	for index, item := range request.Items {
		items[index] = dto.CreateOrderItemInput{ProductID: item.ProductID, Quantity: item.Quantity, UnitPriceMinor: item.UnitPriceMinor}
	}
	return dto.CreateOrderInput{CustomerID: request.CustomerID, Items: items}, nil
}

func decodeError(err error) error {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return apperrors.New(apperrors.PayloadTooLarge, "PAYLOAD_TOO_LARGE", "El cuerpo supera el limite permitido")
	}
	return apperrors.New(apperrors.BadRequest, "INVALID_JSON", "El cuerpo JSON no es valido")
}

func abortWithError(c *gin.Context, err error) {
	_ = c.Error(err)
	c.Abort()
}
