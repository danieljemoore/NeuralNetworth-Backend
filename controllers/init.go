package controllers

import (
	"context"
	"log"
	"os"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var geminiApiKey string

func InitAI() {
	geminiApiKey = os.Getenv("GEMINI_API_KEY")
	if geminiApiKey == "" {
		log.Fatal("gemini_api_key not set in environment")
	}
}

// MongoDB collections
var (
	PortfolioCollection *mongo.Collection
	CompanyCollection   *mongo.Collection
)

// Initialize MongoDB collections
func SetPortfolioCollection(db *mongo.Database) {
	PortfolioCollection = db.Collection("portfolios")

	// Create a unique index on the "player" field
	indexModel := mongo.IndexModel{
		Keys:    bson.M{"player": 1}, // Index on "player" field, ascending
		Options: options.Index().SetUnique(true),
	}
	_, err := PortfolioCollection.Indexes().CreateOne(context.TODO(), indexModel)
	if err != nil {
		log.Fatalf("Failed to create unique index on player field: %v", err)
	}
}

func SetCompanyCollection(db *mongo.Database) {
	CompanyCollection = db.Collection("companies")
}
