package main

import (
	"log"
	"midnight-trader/controllers"
	"midnight-trader/db"
	"midnight-trader/models"
	"midnight-trader/routes"
	"midnight-trader/websocket"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	// Connect to the database
	db.ConnectDB()
	database := db.GetDB()

	// Initialize the WebSocketBroadcaster
	// Initialize the WebSocket hub
	hub := models.NewHub()
	go hub.Run()

	// now we can safely initialize collections

	controllers.InitAI()

	// Initialize all collections
	controllers.SetCompanyCollection(database)
	controllers.SetTradeCollection(database)
	controllers.SetPortfolioCollection(database)
	controllers.SetTransactionsCollection(database)

	// Initialize routes
	r := gin.Default()
	r.GET("/ws", func(c *gin.Context) {
		websocket.ServeWs(hub, c.Writer, c.Request)
	})
	routes.WebSocketRoutes(r)
	routes.CompanyRoutes(r)
	api := r.Group("/api")
	{
		api.GET("/companies", controllers.GetCompaniesHandler)
		api.DELETE("/companies", controllers.ClearData) // <- add this
		api.POST("/generate", controllers.GenerateCompanies)
		api.POST("/generate/data", controllers.GenerateHistoricalData(hub))
		api.POST("/generate/append", controllers.AppendGeneratedHistoricalData(hub))

		api.POST("/portfolio", controllers.CreatePortfolioHandler(hub))
		api.GET("/portfolio", controllers.GetPortfolioHandler(hub))
		api.DELETE("/portfolio", controllers.DeletePortfolioHandler(hub))
		api.GET("/portfolios", controllers.GetPortfoliosHandler())

		api.GET("/trades", controllers.GetTradesHandler())
		api.POST("/trades", controllers.ExecuteTradeHandler(hub, db.Client))

	}
	routes.TradeRoutes(r)
	routes.PortfolioRoutes(r)
	routes.RoundRoutes(r)

	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	log.Println("Server running on port", port)
	r.Run(":" + port)
}
