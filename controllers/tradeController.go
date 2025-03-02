package controllers

import (
	"context"
	"fmt"
	"midnight-trader/db"
	"midnight-trader/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var tradeCollection *mongo.Collection
var transactionsCollection *mongo.Collection

// SetTradeCollection initializes the trades collection
func SetTradeCollection(db *mongo.Database) {
	tradeCollection = db.Collection("trades")
}

func SetTransactionsCollection(db *mongo.Database) {
	transactionsCollection = db.Collection("transactions")
}

// buy stock action with ws event
func BuyStock(client *mongo.Client, ctx context.Context, player, ticker string, quantity int, price float64) error {
	session, err := client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	callback := func(sessCtx mongo.SessionContext) (interface{}, error) {
		if quantity <= 0 {
			return nil, fmt.Errorf("quantity must be positive")
		}
		if price <= 0 {
			return nil, fmt.Errorf("price must be positive")
		}
		if player == "" || ticker == "" {
			return nil, fmt.Errorf("player and ticker must not be empty")
		}
		portfolio, err := GetPortfolio(ctx, player)
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
		return nil, SavePortfolio(sessCtx, portfolio)
	}
	_, err = session.WithTransaction(ctx, callback)
	if err == nil {
		// broadcast buy event with details and updated portfolio
		updatedPortfolio, _ := GetPortfolio(ctx, player)
		go SendRoundUpdate("buy_stock", map[string]interface{}{
			"player":    player,
			"ticker":    ticker,
			"quantity":  quantity,
			"price":     price,
			"portfolio": updatedPortfolio,
		})
		// Fetch and broadcast all portfolios
		portfolios, err := GetPortfolios(ctx)
		if err == nil {
			go SendRoundUpdate("all_portfolios", portfolios)
		}

	}
	return err
}

// sell stock action with ws event
func SellStock(ctx context.Context, player, ticker string, quantity int, price float64) error {
	if quantity <= 0 {
		return fmt.Errorf("quantity must be positive")
	}
	if price <= 0 {
		return fmt.Errorf("price must be positive")
	}
	if player == "" || ticker == "" {
		return fmt.Errorf("player and ticker must not be empty")
	}
	portfolio, err := GetPortfolio(ctx, player)
	if err != nil {
		return err
	}
	if portfolio.Companies[ticker] < quantity {
		return fmt.Errorf("not enough stock to sell")
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
		return err
	}
	err = SavePortfolio(ctx, portfolio)
	if err == nil {
		go SendRoundUpdate("sell_stock", map[string]interface{}{
			"player":    player,
			"ticker":    ticker,
			"quantity":  quantity,
			"price":     price,
			"portfolio": portfolio,
		})
		// Fetch and broadcast all portfolios
		portfolios, err := GetPortfolios(ctx)
		if err == nil {
			go SendRoundUpdate("all_portfolios", portfolios)
		}

	}
	return err
}

// GetTrades retrieves trades, optionally filtered by player
func GetTrades(c *gin.Context) {
	player := c.Query("player")
	filter := bson.M{}
	if player != "" {
		filter["player"] = player
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var trades []models.Trade
	cursor, err := tradeCollection.Find(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var trade models.Trade
		if err := cursor.Decode(&trade); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode trade data"})
			return
		}
		trades = append(trades, trade)
	}

	c.JSON(http.StatusOK, trades)
}

func GetTradesHandler(c *gin.Context) {
	player := c.Query("player")
	filter := bson.M{}
	if player != "" {
		filter["player"] = player
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	var trades []models.Trade
	cursor, err := tradeCollection.Find(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
		var trade models.Trade
		if err := cursor.Decode(&trade); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode trade data"})
			return
		}
		trades = append(trades, trade)
	}
	c.JSON(http.StatusOK, trades)
}

// ExecuteTrade executes a buy or sell trade and updates the portfolio
func ExecuteTrade(c *gin.Context) {
	var trade models.Trade
	if err := c.ShouldBindJSON(&trade); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid trade data: " + err.Error()})
		return
	}

	// Basic validation
	if trade.Player == "" || trade.Ticker == "" || trade.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Player, ticker, and amount are required and amount must be positive"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fetch company to get current stock price
	var company models.Company
	err := companyCollection.FindOne(ctx, bson.M{"ticker": trade.Ticker}).Decode(&company)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}

	trade.Price = company.StockPrice
	trade.Timestamp = time.Now()

	// Execute the trade
	var tradeErr error
	switch trade.Type {
	case "buy":
		tradeErr = BuyStock(db.Client, ctx, trade.Player, trade.Ticker, trade.Amount, trade.Price)
	case "sell":
		tradeErr = SellStock(ctx, trade.Player, trade.Ticker, trade.Amount, trade.Price)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid trade type, must be 'buy' or 'sell'"})
		return
	}

	if tradeErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": tradeErr.Error()})
		return
	}

	// Log the trade
	_, err = tradeCollection.InsertOne(ctx, trade)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to log trade: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Trade executed successfully", "trade": trade})
}

func ExecuteTradeHandler(c *gin.Context) {
	var trade models.Trade
	if err := c.ShouldBindJSON(&trade); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid trade data: " + err.Error()})
		return
	}
	if trade.Player == "" || trade.Ticker == "" || trade.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Player, ticker, and amount are required and amount must be positive"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	var company models.Company
	err := companyCollection.FindOne(ctx, bson.M{"ticker": trade.Ticker}).Decode(&company)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Company not found"})
		return
	}
	trade.Price = company.StockPrice
	trade.Timestamp = time.Now()
	var tradeErr error
	switch trade.Type {
	case "buy":
		tradeErr = BuyStock(db.Client, ctx, trade.Player, trade.Ticker, trade.Amount, trade.Price)
	case "sell":
		tradeErr = SellStock(ctx, trade.Player, trade.Ticker, trade.Amount, trade.Price)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid trade type, must be 'buy' or 'sell'"})
		return
	}
	if tradeErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": tradeErr.Error()})
		return
	}
	_, err = tradeCollection.InsertOne(ctx, trade)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to log trade: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Trade executed successfully", "trade": trade})
}

func DeleteTrades(ctx context.Context) error {
	_, err := tradeCollection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to delete trades: %v", err)
	}
	return nil
}

func DeleteTradesHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	err := DeleteTrades(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "all trades deleted"})
}
