package controllers

import (
	"context"
	"fmt"
	"log"
	"midnight-trader/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// mongo collections
var portfolioCollection *mongo.Collection

// set collection functions
func SetPortfolioCollection(db *mongo.Database) {
	portfolioCollection = db.Collection("portfolios")
	// Create a unique index on the "player" field
	indexModel := mongo.IndexModel{
		Keys:    bson.M{"player": 1}, // Index on "player" field, ascending
		Options: options.Index().SetUnique(true),
	}
	_, err := portfolioCollection.Indexes().CreateOne(context.TODO(), indexModel)
	if err != nil {
		log.Fatalf("Failed to create unique index on player field: %v", err)
	}
}

func CreatePortfolio(ctx context.Context, player string) error {
	// Check if a portfolio already exists for the player
	var existing models.Portfolio
	err := portfolioCollection.FindOne(ctx, bson.M{"player": player}).Decode(&existing)
	if err == nil {
		return fmt.Errorf("portfolio already exists for player %s", player)
	}
	if err != mongo.ErrNoDocuments {
		return fmt.Errorf("error checking for existing portfolio: %v", err)
	}

	// No existing portfolio, create a new one
	portfolio := models.Portfolio{
		Player:    player,
		Funds:     10000.0,
		Companies: make(map[string]int),
	}
	_, err = portfolioCollection.InsertOne(ctx, portfolio)
	if err != nil {
		return fmt.Errorf("failed to create new portfolio: %v", err)
	}
	// Send the event only when a new portfolio is created
	go SendRoundUpdate("player_joined", portfolio)
	return nil
}

func CreatePortfolioHandler(c *gin.Context) {
	player := c.Query("player")
	if player == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "player parameter is required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	err := CreatePortfolio(ctx, player)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "portfolio created"})
}

// get portfolio action, creates new portfolio if not exist and broadcasts event
func GetPortfolio(ctx context.Context, player string) (*models.Portfolio, error) {
	var portfolio models.Portfolio
	// Try to find the existing portfolio
	filter := bson.M{"player": player}
	err := portfolioCollection.FindOne(ctx, filter).Decode(&portfolio)
	if err == nil {
		// Portfolio exists, return it without sending an event
		return &portfolio, nil
	}
	if err != mongo.ErrNoDocuments {
		return nil, fmt.Errorf("failed to fetch portfolio: %v", err)
	}

	// Portfolio doesn’t exist, create it
	portfolio = models.Portfolio{
		Player:    player,
		Funds:     10000.0,
		Companies: make(map[string]int),
	}
	_, err = portfolioCollection.InsertOne(ctx, portfolio)
	if err != nil {
		return nil, fmt.Errorf("failed to create new portfolio: %v", err)
	}
	// Send the event only when a new portfolio is created
	go SendRoundUpdate("player_joined", portfolio)
	return &portfolio, nil
}

// handler for getting portfolio with ws event
func GetPortfolioHandler(c *gin.Context) {
	player := c.Query("player")
	if player == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "player parameter is required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	portfolio, err := GetPortfolio(ctx, player)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	fmt.Printf("portfolio for %s: %+v\n", player, portfolio)
	c.Header("Content-Type", "application/json") // Corrected content type
	c.JSON(http.StatusOK, portfolio)
}

func GetPortfolios(ctx context.Context) ([]models.Portfolio, error) {
	var portfolios []models.Portfolio
	cursor, err := portfolioCollection.Find(ctx, bson.M{})
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
	return portfolios, nil
}

func GetPortfoliosHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	portfolios, err := GetPortfolios(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, portfolios)
}

// save portfolio
func SavePortfolio(ctx context.Context, portfolio *models.Portfolio) error {
	filter := bson.M{"player": portfolio.Player}
	_, err := portfolioCollection.ReplaceOne(ctx, filter, portfolio)
	if err != nil {
		return fmt.Errorf("failed to save portfolio: %v", err)
	}
	return nil
}
func DeletePortfolio(ctx context.Context, player string) error {
	filter := bson.M{"player": player}
	_, err := portfolioCollection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete portfolio: %v", err)
	}
	return nil
}

// handler for deleting portfolio
func DeletePortfolioHandler(c *gin.Context) {
	player := c.Query("player")
	if player == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "player parameter is required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	err := DeletePortfolio(ctx, player)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "portfolio deleted"})
}

func DeletePortfolios(ctx context.Context) error {
	_, err := portfolioCollection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to delete portfolios: %v", err)
	}
	return nil
}

func DeletePortfoliosHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	err := DeletePortfolios(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "portfolios deleted"})
}

// log trade transaction
func LogTransaction(ctx context.Context, trade models.Trade) error {
	_, err := transactionsCollection.InsertOne(ctx, trade)
	if err != nil {
		return fmt.Errorf("failed to log transaction: %v", err)
	}
	return nil
}
