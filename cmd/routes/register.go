package routes

import (
	"io"
	"net/http"
	"time"

	"eventflow/internal/health/presentation"
	"eventflow/internal/order/presentation/handler"
	orderrouter "eventflow/internal/order/presentation/router"
	"eventflow/pkg/httputil"
	"eventflow/pkg/metrics"
	"eventflow/pkg/middleware"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type dependencies struct {
	orderHandler   *handler.OrderHandler
	readiness      presentation.ReadinessExecutor
	metrics        *metrics.Registry
	metricsHandler http.Handler
}

// Option configura rutas opcionales según las dependencias disponibles.
type Option func(*dependencies)

// WithOrderHandler registra las rutas del módulo de pedidos.
func WithOrderHandler(orderHandler *handler.OrderHandler) Option {
	return func(dependencies *dependencies) { dependencies.orderHandler = orderHandler }
}

// WithReadiness registra la sonda de disponibilidad.
func WithReadiness(readiness presentation.ReadinessExecutor) Option {
	return func(dependencies *dependencies) { dependencies.readiness = readiness }
}

// WithMetrics registra la exposición local de métricas.
func WithMetrics(registry *metrics.Registry, metricsHandler http.Handler) Option {
	return func(dependencies *dependencies) {
		dependencies.metrics = registry
		dependencies.metricsHandler = metricsHandler
	}
}

// New crea el router operativo sin los middlewares por defecto de Gin para
// conservar una única respuesta sanitizada para errores y panics.
func New(log *zap.Logger, options ...Option) *gin.Engine {
	if log == nil {
		log = zap.NewNop()
	}
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(middleware.Correlation(log), errorMiddleware(), recoveryMiddleware(log))
	dependencies := dependencies{}
	for _, option := range options {
		option(&dependencies)
	}
	if dependencies.metrics != nil {
		router.Use(middleware.HTTPMetrics(dependencies.metrics))
	}
	presentation.Register(router, dependencies.readiness)
	if dependencies.metricsHandler != nil {
		router.GET("/metrics", gin.WrapH(dependencies.metricsHandler))
	}
	if dependencies.orderHandler != nil {
		orderrouter.Register(router, dependencies.orderHandler)
	}
	return router
}

func errorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if last := c.Errors.Last(); last != nil && !c.Writer.Written() {
			_ = httputil.WriteError(c.Writer, last.Err, "", time.Now())
		}
	}
}

func recoveryMiddleware(log *zap.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, recovered any) {
		if c.Writer.Written() {
			return
		}
		log.Error("panic recuperado")
		_ = httputil.WriteError(c.Writer, nil, "", time.Now())
		c.Abort()
	})
}
