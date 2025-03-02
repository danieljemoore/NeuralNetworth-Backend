package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"midnight-trader/models"
)

func extractAIResponse(result map[string]interface{}) (string, error) {
	candidates, ok := result["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return "", fmt.Errorf("no candidates found in AI response")
	}

	firstCandidate, ok := candidates[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid candidate format")
	}

	content, ok := firstCandidate["content"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("no content found in candidate")
	}

	parts, ok := content["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return "", fmt.Errorf("no parts found in content")
	}

	firstPart, ok := parts[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid part format")
	}

	text, ok := firstPart["text"].(string)
	if !ok {
		return "", fmt.Errorf("no text found in part")
	}

	return text, nil
}
func GenerateCompanies(c *gin.Context) {
	// context for mongodb ops
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// drop old companies
	if err := CompanyCollection.Drop(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to drop existing companies: " + err.Error()})
		return
	}

	// generate companies via gemini
	prompt := `generate data for 9 fictional companies for a stock trading game.
for each company, provide:
- company name (realistic sounding tech or pharma company name)
- company ticker (3-4 letter abbreviation)
- short description (1-2 sentences describing their business)
- starting stock price (a realistic stock price as a floating point number)

format the response as a json array of objects. each object should have the keys: "name", "ticker", "description", "stockPrice".`

	reqBody, err := json.Marshal(map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to marshal request body: " + err.Error()})
		return
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=%s", geminiApiKey)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch ai-generated companies: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read response body: " + err.Error()})
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse ai response: " + err.Error()})
		return
	}

	log.Println("gemini api response:", string(body))

	responseText, err := extractAIResponse(result)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to extract AI response: " + err.Error()})
		return
	}

	// clean up code fences (note newline sensitivity)
	cleanedJSON := strings.TrimPrefix(responseText, "```json\n")
	cleanedJSON = strings.TrimSuffix(cleanedJSON, "```\n")
	cleanedJSON = strings.TrimSpace(cleanedJSON)

	var companiesData []map[string]interface{}
	if err := json.Unmarshal([]byte(cleanedJSON), &companiesData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse cleaned json: " + err.Error()})
		return
	}

	// convert to models.Company
	var companies []models.Company
	for _, comp := range companiesData {
		name, _ := comp["name"].(string)
		ticker, _ := comp["ticker"].(string)
		description, _ := comp["description"].(string)
		var stockPrice float64
		switch v := comp["stockPrice"].(type) {
		case float64:
			stockPrice = v
		case string:
			stockPrice, err = strconv.ParseFloat(v, 64)
			if err != nil {
				stockPrice = 0
			}
		default:
			stockPrice = 0
		}
		companies = append(companies, models.Company{
			Name:                  name,
			Ticker:                ticker,
			Description:           description,
			StockPrice:            stockPrice,
			HistoricalStockPrices: []float64{stockPrice},
		})
	}

	// insert into mongodb
	companyDocs := make([]interface{}, len(companies))
	for i, company := range companies {
		companyDocs[i] = company
	}

	_, err = CompanyCollection.InsertMany(ctx, companyDocs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to insert companies: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "new companies generated", "companies": companies})
}

func GenerateHistoricalData(hub *models.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
		defer cancel()

		// fetch existing companies from mongo
		var companies []models.Company
		cursor, err := CompanyCollection.Find(ctx, bson.M{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch companies: " + err.Error()})
			return
		}
		defer cursor.Close(ctx)
		for cursor.Next(ctx) {
			var comp models.Company
			if err := cursor.Decode(&comp); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode company: " + err.Error()})
				return
			}
			companies = append(companies, comp)
		}
		if err := cursor.Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "cursor error: " + err.Error()})
			return
		}

		// marshal companies to json string for prompt
		companiesData, err := json.Marshal(companies)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to marshal companies: " + err.Error()})
			return
		}

		// build gemini prompt using existing companies data
		prompt := fmt.Sprintf(`generate historical stock price data for a stock trading game.
the input is a json array of companies with keys "name", "ticker", "description", "stockPrice", "historicalStockPrices".
for each company, generate an array of 10 historical prices (floating point numbers).
dates don't matter. Make sure there are winners and losers. Some companies must FAIL badly. Some will make people very rich. We want to see a variety of price movements between the stocks.
format the response as a json object where each key is a company ticker and the value is the array of prices.
here's the companies data: %s`, string(companiesData))

		reqBody, err := json.Marshal(map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"parts": []map[string]string{
						{"text": prompt},
					},
				},
			},
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to marshal request body: " + err.Error()})
			return
		}

		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=%s", geminiApiKey)
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqBody))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch ai-generated historical data: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read response body: " + err.Error()})
			return
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse ai response: " + err.Error()})
			return
		}

		// Usage within your handler:
		responseText, err := extractAIResponse(result)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to extract AI response: " + err.Error()})
			return
		}

		// remove code fences if present
		cleanedJSON := strings.TrimPrefix(responseText, "```json\n")
		cleanedJSON = strings.TrimSuffix(cleanedJSON, "```\n")
		cleanedJSON = strings.TrimSpace(cleanedJSON)

		// expected format: map[ticker][]float64
		var historicalData map[string][]float64
		if err := json.Unmarshal([]byte(cleanedJSON), &historicalData); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse historical data json: " + err.Error()})
			return
		}

		// Update each company with its historical stock prices and update stockPrice to the latest price
		for ticker, prices := range historicalData {
			if len(prices) == 0 {
				log.Printf("no historical prices provided for ticker %s", ticker)
				continue
			}
			latestPrice := prices[len(prices)-1]
			message := models.WSMessage{
				Event: "stock_update",
				Data: map[string]interface{}{
					"ticker": ticker,
					"price":  latestPrice,
				},
			}
			hub.Broadcast <- message
			filter := bson.M{"ticker": ticker}
			update := bson.M{
				"$set": bson.M{
					"historicalStockPrices": prices,
					"stockPrice":            latestPrice,
				},
			}
			res, err := CompanyCollection.UpdateOne(ctx, filter, update)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update company " + ticker + ": " + err.Error()})
				return
			}
			if res.ModifiedCount == 0 {
				log.Printf("no document updated for ticker %s", ticker)
			}
		}

		c.JSON(http.StatusOK, gin.H{"message": "historical data updated", "data": historicalData})
	}
}

func AppendGeneratedHistoricalData(hub *models.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// fetch existing companies from mongo
		var companies []models.Company
		cursor, err := CompanyCollection.Find(ctx, bson.M{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch companies: " + err.Error()})
			return
		}
		defer cursor.Close(ctx)
		for cursor.Next(ctx) {
			var comp models.Company
			if err := cursor.Decode(&comp); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode company: " + err.Error()})
				return
			}
			companies = append(companies, comp)
		}
		if err := cursor.Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "cursor error: " + err.Error()})
			return
		}

		// marshal companies to json string for prompt
		companiesData, err := json.Marshal(companies)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to marshal companies: " + err.Error()})
			return
		}

		// build gemini prompt using existing companies data
		prompt := fmt.Sprintf(`generate additional historical stock price data for a stock trading game.
	the input is a json array of companies with keys "name", "ticker", "description", "stockPrice", "historicalStockPrices".
	for each company, generate an array of 10 historical prices (floating point numbers).
	dates don't matter. Make sure there are winners and losers. Some companies must FAIL badly. Some will make people very rich. Try not to repeat the same prices, we don't want heavy seasonality within 10 days! We want to see a variety of price movements between the stocks. If the ending price is not change greater than $50 of the last appended price, we will consider the data as not updated, Also a bus of children will be exploded.
	format the response as a json object where each key is a company ticker and the value is the array of prices.
	here's the companies data: %s`, string(companiesData))

		reqBody, err := json.Marshal(map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"parts": []map[string]string{
						{"text": prompt},
					},
				},
			},
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to marshal request body: " + err.Error()})
			return
		}

		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-flash:generateContent?key=%s", geminiApiKey) // fix here
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqBody))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch ai-generated historical data: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read response body: " + err.Error()})
			return
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse ai response: " + err.Error()})
			return
		}

		// Usage within your handler:
		responseText, err := extractAIResponse(result)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to extract AI response: " + err.Error()})
			return
		}

		// remove code fences if present
		cleanedJSON := strings.TrimPrefix(responseText, "```json\n")
		cleanedJSON = strings.TrimSuffix(cleanedJSON, "```\n")
		cleanedJSON = strings.TrimSpace(cleanedJSON)

		// expected format: map[ticker][]float64
		var historicalData map[string][]float64
		if err := json.Unmarshal([]byte(cleanedJSON), &historicalData); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse historical data json: " + err.Error()})
			return
		}

		// Update each company with its historical stock prices and update stockPrice to the latest appended price
		for ticker, prices := range historicalData {
			if len(prices) == 0 {
				log.Printf("no historical prices provided for ticker %s", ticker)
				continue
			}
			latestAppendedPrice := prices[len(prices)-1]

			filter := bson.M{"ticker": ticker}
			update := bson.M{
				"$push": bson.M{"historicalStockPrices": bson.M{"$each": prices}},
				"$set": bson.M{
					"stockPrice": latestAppendedPrice,
				},
			}
			res, err := CompanyCollection.UpdateOne(ctx, filter, update)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update company " + ticker + ": " + err.Error()})
				return
			}
			if res.ModifiedCount == 0 {
				log.Printf("no document updated for ticker %s", ticker)
			}
			// Emit stock_update event
			message := models.WSMessage{
				Event: "stock_update",
				Data: map[string]interface{}{
					"ticker": ticker,
					"price":  latestAppendedPrice,
				},
			}
			hub.Broadcast <- message

		}

		c.JSON(http.StatusOK, gin.H{"message": "historical data appended", "data": historicalData})
	}
}
