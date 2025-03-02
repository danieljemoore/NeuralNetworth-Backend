package routes

import (
	"github.com/gin-gonic/gin"
	"midnight-trader/controllers"
)

func RoundRoutes(r *gin.Engine) {
	r.GET("/api/rounds", controllers.GetRoundStatusHandler)
	r.POST("/api/rounds", controllers.StartRoundHandler)
	r.DELETE("/api/rounds", controllers.EndRoundHandler)
}
