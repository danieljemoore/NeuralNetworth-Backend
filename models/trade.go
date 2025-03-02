package models

import (
	"time"
)

type Trade struct {
	Player    string    `json:"player" bson:"player"`
	Company   string    `json:"company" bson:"company"`
	Ticker    string    `json:"ticker" bson:"ticker"`
	Type      string    `json:"type" bson:"type"` // "buy" or "sell"
	Amount    int       `json:"amount" bson:"amount"`
	Price     float64   `json:"price" bson:"price"`
	Timestamp time.Time `json:"timestamp" bson:"timestamp"`
}
