package routes

import (
	"github.com/gin-gonic/gin"
	"midnight-trader/controllers"
)

func WebSocketRoutes(r *gin.Engine) {
	r.GET("/ws", controllers.WebSocketHandler)
}
