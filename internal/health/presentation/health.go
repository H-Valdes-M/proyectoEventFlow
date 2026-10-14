package presentation

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ReadinessExecutor es el puerto del handler de readiness.
type ReadinessExecutor interface {
	Execute(context.Context) error
}

// Register publica los probes operativos disponibles en este hito.
func Register(router gin.IRoutes, readiness ReadinessExecutor) {
	router.GET("/healthz", liveness)
	if readiness != nil {
		router.GET("/readyz", ready(readiness))
	}
}

func liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func ready(readiness ReadinessExecutor) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := readiness.Execute(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
