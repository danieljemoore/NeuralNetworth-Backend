// controllers/controllers.go
package controllers

import (
	"context"
	"fmt"
	"midnight-trader/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// CreatePortfolio creates a new portfolio for a player if it doesn't exist.
func CreatePortfolio(ctx context.Context, player string) (*models.Portfolio, error) {
	// Check if a portfolio already exists for the player
	var existing models.Portfolio
	err := PortfolioCollection.FindOne(ctx, bson.M{"player": player}).Decode(&existing)
	if err == nil {
		return nil, fmt.Errorf("portfolio already exists for player %s", player)
	}
	if err != mongo.ErrNoDocuments {
		return nil, fmt.Errorf("error checking for existing portfolio: %v", err)
	}

	// No existing portfolio, create a new one
	portfolio := &models.Portfolio{
		Player:    player,
		Funds:     10000.0,
		Companies: make(map[string]int),
	}
	_, err = PortfolioCollection.InsertOne(ctx, portfolio)
	if err != nil {
		return nil, fmt.Errorf("failed to create new portfolio: %v", err)
	}

	return portfolio, nil
}

// CreatePortfolioHandler handles the creation of a new portfolio and broadcasts the event.
func CreatePortfolioHandler(hub *models.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		player := c.Query("player")
		if player == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "player parameter is required"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		portfolio, err := CreatePortfolio(ctx, player)
		if err != nil {
			if err.Error() == fmt.Sprintf("portfolio already exists for player %s", player) {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "portfolio created", "portfolio": portfolio})

		// Broadcast the "portfolio_created" event
		message := models.WSMessage{
			Event: "portfolio_created",
			Data:  player,
		}
		hub.Broadcast <- message
	}
}

// GetPortfolio retrieves a player's portfolio, creating one if it doesn't exist, and broadcasts the event if created.
func GetPortfolio(ctx context.Context, player string) (*models.Portfolio, bool, error) {
	var portfolio models.Portfolio
	// Attempt to find the existing portfolio
	filter := bson.M{"player": player}
	err := PortfolioCollection.FindOne(ctx, filter).Decode(&portfolio)
	if err == nil {
		// Portfolio exists
		return &portfolio, false, nil
	}
	if err != mongo.ErrNoDocuments {
		return nil, false, fmt.Errorf("failed to fetch portfolio: %v", err)
	}

	// Portfolio doesn't exist, create it
	portfolio = models.Portfolio{
		Player:    player,
		Funds:     10000.0,
		Companies: make(map[string]int),
	}
	_, err = PortfolioCollection.InsertOne(ctx, portfolio)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create new portfolio: %v", err)
	}

	return &portfolio, true, nil
}

// GetPortfolioHandler handles fetching a player's portfolio and broadcasts an event if created.
func GetPortfolioHandler(hub *models.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		player := c.Query("player")
		if player == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "player parameter is required"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		portfolio, isNew, err := GetPortfolio(ctx, player)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.Header("Content-Type", "application/json")
		c.JSON(http.StatusOK, portfolio)

		if isNew {
			// Broadcast the "player_joined" event
			message := models.WSMessage{
				Event: "player_joined",
				Data:  player,
			}
			hub.Broadcast <- message
		}
	}
}

// GetPortfolios retrieves all portfolios.
func GetPortfolios(ctx context.Context) ([]models.Portfolio, error) {
	var portfolios []models.Portfolio
	cursor, err := PortfolioCollection.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch portfolios: %v", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var portfolio models.Portfolio
		if err := cursor.Decode(&portfolio); err != nil {
			return nil, fmt.Errorf("failed to decode portfolio: %v", err)
		}
		portfolios = append(portfolios, portfolio)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %v", err)
	}

	return portfolios, nil
}

// GetPortfoliosHandler handles fetching all portfolios.
func GetPortfoliosHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		portfolios, err := GetPortfolios(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, portfolios)
	}
}

// SavePortfolio updates an existing portfolio in the database.
func SavePortfolio(ctx context.Context, portfolio *models.Portfolio) error {
	filter := bson.M{"player": portfolio.Player}
	_, err := PortfolioCollection.ReplaceOne(ctx, filter, portfolio)
	if err != nil {
		return fmt.Errorf("failed to save portfolio: %v", err)
	}
	return nil
}

// DeletePortfolio removes a player's portfolio from the database.
func DeletePortfolio(ctx context.Context, player string) error {
	filter := bson.M{"player": player}
	_, err := PortfolioCollection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete portfolio: %v", err)
	}
	return nil
}

// DeletePortfolioHandler handles the deletion of a player's portfolio and broadcasts the event.
func DeletePortfolioHandler(hub *models.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		player := c.Query("player")
		if player == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "player parameter is required"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		err := DeletePortfolio(ctx, player)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "portfolio deleted"})

		// Broadcast the "portfolio_deleted" event
		message := models.WSMessage{
			Event: "portfolio_deleted",
			Data:  player,
		}
		hub.Broadcast <- message
	}
}

// LogTransaction logs a trade transaction in the database.
func LogTransaction(ctx context.Context, trade models.Trade) error {
	_, err := transactionsCollection.InsertOne(ctx, trade)
	if err != nil {
		return fmt.Errorf("failed to log transaction: %v", err)
	}
	return nil
}
