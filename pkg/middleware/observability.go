// Package middleware contiene observabilidad transversal para HTTP.
package middleware

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"time"

	"eventflow/pkg/correlation"
	"eventflow/pkg/metrics"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const requestIDHeader = "X-Request-ID"

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// Correlation valida o crea un identificador de solicitud, lo devuelve al cliente
// y registra solo metadatos seguros al finalizar la petición.
func Correlation(log *zap.Logger) gin.HandlerFunc {
	if log == nil {
		log = zap.NewNop()
	}
	return func(c *gin.Context) {
		requestID := c.GetHeader(requestIDHeader)
		if !requestIDPattern.MatchString(requestID) {
			requestID = newRequestID()
		}
		c.Header(requestIDHeader, requestID)
		c.Request = c.Request.WithContext(correlation.WithID(c.Request.Context(), requestID))
		started := time.Now()
		c.Next()
		orderID, _ := c.Get("order_id")
		log.Info("solicitud HTTP completada", zap.String("request_id", requestID), zap.String("correlation_id", requestID), zap.String("order_id", fmt.Sprint(orderID)), zap.String("operation", c.FullPath()), zap.String("method", c.Request.Method), zap.Int("status", c.Writer.Status()), zap.Duration("latency", time.Since(started)))
	}
}

// HTTPMetrics registra resultados y latencia con etiquetas de cardinalidad acotada.
func HTTPMetrics(registry *metrics.Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		registry.ObserveRequest(c.Request.Method, c.FullPath(), c.Writer.Status(), time.Since(started))
	}
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
