package models

type Company struct {
	Name                  string    `json:"name" bson:"name"`
	Ticker                string    `json:"ticker" bson:"ticker"`
	Description           string    `json:"description" bson:"description"`
	StockPrice            float64   `json:"stockPrice" bson:"stockPrice"`
	HistoricalStockPrices []float64 `json:"historicalStockPrices" bson:"historicalStockPrices"`
}
