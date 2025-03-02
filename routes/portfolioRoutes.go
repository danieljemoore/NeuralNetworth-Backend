package routes

import (
	"github.com/gin-gonic/gin"
	"midnight-trader/controllers"
)

func PortfolioRoutes(r *gin.Engine) {
	r.POST("/api/portfolio", controllers.CreatePortfolioHandler)
	r.GET("/api/portfolio", controllers.GetPortfolioHandler)
	r.DELETE("/api/portfolio", controllers.DeletePortfolioHandler)
	r.GET("/api/portfolios", controllers.GetPortfoliosHandler)
	r.DELETE("/api/portfolios", controllers.DeletePortfoliosHandler)
}
