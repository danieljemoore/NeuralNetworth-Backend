package main

import (
	"log"
	"midnight-trader/controllers"
	"midnight-trader/db"
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
	go websocket.WebSocketBroadcaster()

	// now we can safely initialize collections

	controllers.InitAI()

	// Initialize all collections
	controllers.SetCompanyCollection(database)
	controllers.SetTradeCollection(database)
	controllers.SetPortfolioCollection(database)
	controllers.SetTransactionsCollection(database)

	// Initialize routes
	r := gin.Default()
	routes.WebSocketRoutes(r)
	routes.CompanyRoutes(r)
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
