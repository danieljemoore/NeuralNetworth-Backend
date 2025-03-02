package main

import (
	"log"
	"midnight-trader/controllers"
	"midnight-trader/db"
	"midnight-trader/models"
	"midnight-trader/routes"
	"midnight-trader/websocket"
	"net/http"
	"os"
	"time"

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
	// Initialize RoundManager and assign to global for access in controllers.
	roundManager := controllers.NewRoundManager(hub, 30*time.Second, 5) // Example: 30-second rounds, total 5 rounds
	controllers.CurrentRoundManager = roundManager
	// Initialize the RoundController
	roundController := controllers.NewRoundController(roundManager, hub)

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

		api.GET("/round/status", roundController.GetRoundStatus)
		api.POST("/round/start", roundController.StartRound)
		api.POST("/round/end", roundController.EndRound)
		api.POST("/round/join", roundController.JoinRound)
		api.POST("/round/update", roundController.UpdatePortfolio)

		// Note: StartRound and EndRound are now managed by RoundManager
		// You can still provide endpoints to manually control rounds if desired
		// For example:
		api.POST("/round/start_manual", func(c *gin.Context) {
			roundManager.StartNextRound()
			c.JSON(http.StatusOK, gin.H{"message": "manual round start triggered"})
		})
		api.POST("/round/end_manual", func(c *gin.Context) {
			roundManager.EndRound()
			c.JSON(http.StatusOK, gin.H{"message": "manual round end triggered"})
		})

	}
	routes.TradeRoutes(r)
	routes.PortfolioRoutes(r)
	routes.RoundRoutes(r)

	roundManager.Start()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	// Add this to your Gin routes instead of using http.HandleFunc
	r.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})
	log.Println("Server running on port", port)
	r.Run("0.0.0.0:" + port)
}
