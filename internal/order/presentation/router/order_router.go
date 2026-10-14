package router

import (
	"eventflow/internal/order/presentation/handler"
	"github.com/gin-gonic/gin"
)

// Register publica las rutas HTTP del módulo de pedidos.
func Register(routes gin.IRoutes, handler *handler.OrderHandler) {
	routes.POST("/orders", handler.Create)
	routes.GET("/orders/:id", handler.GetByID)
}
