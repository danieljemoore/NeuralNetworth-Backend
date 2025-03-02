package controllers

import (
	"midnight-trader/models"

	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var companyCollection *mongo.Collection

func SetCompanyCollection(db *mongo.Database) {
	companyCollection = db.Collection("companies")
}

func GetCompanies(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var companies []models.Company
	cursor, err := companyCollection.Find(ctx, bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var company models.Company
		cursor.Decode(&company)
		companies = append(companies, company)
	}

	c.JSON(http.StatusOK, companies)
}
func GetCompaniesHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var companies []models.Company
	cursor, err := companyCollection.Find(ctx, bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var company models.Company
		cursor.Decode(&company)
		companies = append(companies, company)
	}

	c.JSON(http.StatusOK, companies)
}

func ClearData(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := companyCollection.Drop(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear companies"})
		return
	}

	err = tradeCollection.Drop(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear trades"})
		return
	}

	err = portfolioCollection.Drop(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear portfolios"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "All game data cleared"})
}
