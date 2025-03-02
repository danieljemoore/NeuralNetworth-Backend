package routes

import (
	"midnight-trader/controllers"

	"github.com/gin-gonic/gin"
)

func CompanyRoutes(r *gin.Engine) {
	r.GET("/api/companies", controllers.GetCompaniesHandler)
	r.DELETE("/api/companies", controllers.ClearData) // <- add this
	r.POST("/api/generate", controllers.GenerateCompanies)
	r.POST("/api/generate/data", controllers.GenerateHistoricalData)
	r.POST("/api/generate/append", controllers.AppendGeneratedHistoricalData)

}
