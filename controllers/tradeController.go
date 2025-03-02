// controllers/tradeController.go
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

// MongoDB collections
var (
	tradeCollection        *mongo.Collection
	transactionsCollection *mongo.Collection
)

// Initialize MongoDB collections
func SetTradeCollection(db *mongo.Database) {
	tradeCollection = db.Collection("trades")
}

func SetTransactionsCollection(db *mongo.Database) {
	transactionsCollection = db.Collection("transactions")
}

// WSMessage represents the structure of WebSocket messages
type WSMessage struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// BuyStock performs the buy operation and returns the updated portfolio
func BuyStock(ctx context.Context, player, ticker string, quantity int, price float64) (*models.Portfolio, error) {
	if quantity <= 0 {
		return nil, fmt.Errorf("quantity must be positive")
	}
	if price <= 0 {
		return nil, fmt.Errorf("price must be positive")
	}
	if player == "" || ticker == "" {
		return nil, fmt.Errorf("player and ticker must not be empty")
	}

	portfolio, _, err := GetPortfolio(ctx, player)
	if err != nil {
		return nil, err
	}

	totalCost := price * float64(quantity)
	if portfolio.Funds < totalCost {
		return nil, fmt.Errorf("insufficient funds")
	}

	portfolio.Funds -= totalCost
	if portfolio.Companies == nil {
		portfolio.Companies = make(map[string]int)
	}
	portfolio.Companies[ticker] += quantity

	trade := models.Trade{
		Player:    player,
		Ticker:    ticker,
		Type:      "buy",
		Amount:    quantity,
		Price:     price,
		Timestamp: time.Now(),
	}

	if err := LogTransaction(ctx, trade); err != nil {
		return nil, err
	}

	if err := SavePortfolio(ctx, portfolio); err != nil {
		return nil, err
	}

	return portfolio, nil
}

// SellStock performs the sell operation and returns the updated portfolio
func SellStock(ctx context.Context, player, ticker string, quantity int, price float64) (*models.Portfolio, error) {
	if quantity <= 0 {
		return nil, fmt.Errorf("quantity must be positive")
	}
	if price <= 0 {
		return nil, fmt.Errorf("price must be positive")
	}
	if player == "" || ticker == "" {
		return nil, fmt.Errorf("player and ticker must not be empty")
	}

	portfolio, _, err := GetPortfolio(ctx, player)
	if err != nil {
		return nil, err
	}

	currentQuantity, exists := portfolio.Companies[ticker]
	if !exists || currentQuantity < quantity {
		return nil, fmt.Errorf("not enough stock to sell")
	}

	totalRevenue := price * float64(quantity)
	portfolio.Funds += totalRevenue
	portfolio.Companies[ticker] -= quantity
	if portfolio.Companies[ticker] == 0 {
		delete(portfolio.Companies, ticker)
	}

	trade := models.Trade{
		Player:    player,
		Ticker:    ticker,
		Type:      "sell",
		Amount:    quantity,
		Price:     price,
		Timestamp: time.Now(),
	}

	if err := LogTransaction(ctx, trade); err != nil {
		return nil, err
	}

	if err := SavePortfolio(ctx, portfolio); err != nil {
		return nil, err
	}

	return portfolio, nil
}

// ExecuteTradeHandler handles executing a trade (buy/sell) and broadcasting events
func ExecuteTradeHandler(hub *models.Hub, client *mongo.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var trade models.Trade
		if err := c.ShouldBindJSON(&trade); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid trade data: " + err.Error()})
			return
		}

		if trade.Player == "" || trade.Ticker == "" || trade.Amount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Player, ticker, and amount are required and amount must be positive"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		// Fetch company to get current stock price
		var company models.Company
		err := GetCompany(ctx, trade.Ticker, &company)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
			return
		}

		trade.Price = company.StockPrice
		trade.Timestamp = time.Now()

		var updatedPortfolio *models.Portfolio

		if trade.Type == "buy" {
			updatedPortfolio, err = BuyStock(ctx, trade.Player, trade.Ticker, trade.Amount, trade.Price)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		} else if trade.Type == "sell" {
			updatedPortfolio, err = SellStock(ctx, trade.Player, trade.Ticker, trade.Amount, trade.Price)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid trade type, must be 'buy' or 'sell'"})
			return
		}

		// Log the trade (already done in Buy/Sell functions, so this might be redundant)
		// Uncomment if you want to log trades separately
		/*
		   _, err = tradeCollection.InsertOne(ctx, trade)
		   if err != nil {
		       c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to log trade: " + err.Error()})
		       return
		   }
		*/

		c.JSON(http.StatusOK, gin.H{"message": "Trade executed successfully", "trade": trade})

		// Broadcast the trade event with details and updated portfolio
		tradeEvent := models.WSMessage{
			Event: "trade_executed",
			Data: map[string]interface{}{
				"player":    trade.Player,
				"ticker":    trade.Ticker,
				"quantity":  trade.Amount,
				"price":     trade.Price,
				"type":      trade.Type,
				"timestamp": trade.Timestamp,
				"portfolio": updatedPortfolio,
			},
		}
		hub.Broadcast <- tradeEvent

		// Fetch and broadcast all portfolios
		portfolios, err := GetPortfolios(ctx)
		if err == nil {
			allPortfoliosEvent := models.WSMessage{
				Event: "all_portfolios",
				Data:  portfolios,
			}
			hub.Broadcast <- allPortfoliosEvent
		}
	}
}

// GetTrades retrieves trades, optionally filtered by player
func GetTrades(ctx context.Context, player string) ([]models.Trade, error) {
	filter := bson.M{}
	if player != "" {
		filter["player"] = player
	}

	var trades []models.Trade
	cursor, err := tradeCollection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch trades: %v", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var trade models.Trade
		if err := cursor.Decode(&trade); err != nil {
			return nil, fmt.Errorf("failed to decode trade data: %v", err)
		}
		trades = append(trades, trade)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %v", err)
	}

	return trades, nil
}

// GetTradesHandler handles fetching trades, optionally filtered by player
func GetTradesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		player := c.Query("player")
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		trades, err := GetTrades(ctx, player)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, trades)
	}
}

// ExecuteTradeHandler handles executing a trade and broadcasting events using the WebSocket Hub
func ExecuteTradeHandlerV2(hub *models.Hub, client *mongo.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var trade models.Trade
		if err := c.ShouldBindJSON(&trade); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid trade data: " + err.Error()})
			return
		}

		if trade.Player == "" || trade.Ticker == "" || trade.Amount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Player, ticker, and amount are required and amount must be positive"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		// Fetch company to get current stock price
		var company models.Company
		err := GetCompany(ctx, trade.Ticker, &company)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
			return
		}

		trade.Price = company.StockPrice
		trade.Timestamp = time.Now()

		var updatedPortfolio *models.Portfolio

		if trade.Type == "buy" {
			updatedPortfolio, err = BuyStock(ctx, trade.Player, trade.Ticker, trade.Amount, trade.Price)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		} else if trade.Type == "sell" {
			updatedPortfolio, err = SellStock(ctx, trade.Player, trade.Ticker, trade.Amount, trade.Price)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid trade type, must be 'buy' or 'sell'"})
			return
		}

		// Log the trade
		_, err = tradeCollection.InsertOne(ctx, trade)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to log trade: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Trade executed successfully", "trade": trade})

		// Broadcast the trade event with details and updated portfolio
		tradeEvent := models.WSMessage{
			Event: "trade_executed",
			Data: map[string]interface{}{
				"player":    trade.Player,
				"ticker":    trade.Ticker,
				"quantity":  trade.Amount,
				"price":     trade.Price,
				"type":      trade.Type,
				"timestamp": trade.Timestamp,
				"portfolio": updatedPortfolio,
			},
		}
		hub.Broadcast <- tradeEvent

		// Fetch and broadcast all portfolios
		portfolios, err := GetPortfolios(ctx)
		if err == nil {
			allPortfoliosEvent := models.WSMessage{
				Event: "all_portfolios",
				Data:  portfolios,
			}
			hub.Broadcast <- allPortfoliosEvent
		}
	}
}

// GetTradesHandler handles fetching trades, optionally filtered by player
func GetTradesHandlerV2() gin.HandlerFunc {
	return func(c *gin.Context) {
		player := c.Query("player")
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()

		trades, err := GetTrades(ctx, player)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, trades)
	}
}

// GetCompany fetches a company by ticker
func GetCompany(ctx context.Context, ticker string, company *models.Company) error {
	return CompanyCollection.FindOne(ctx, bson.M{"ticker": ticker}).Decode(company)
}

// Additional functions like DeleteTrade, UpdateTrade can be implemented similarly with consistent WebSocket messaging
