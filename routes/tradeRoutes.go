package routes

import (
	"midnight-trader/controllers"

	"github.com/gin-gonic/gin"
)

func TradeRoutes(r *gin.Engine) {
	r.GET("/api/trades", controllers.GetTradesHandler)
	r.POST("/api/trades", controllers.ExecuteTradeHandler)
	r.DELETE("/api/trades", controllers.DeleteTradesHandler)
}
