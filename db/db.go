package db

import (
	"context"
	"crypto/tls"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var Client *mongo.Client

func ConnectDB() {
	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		log.Fatal("MONGODB_URI not set")
	}

	// Set up MongoDB client options with TLS configuration
	tlsConfig := &tls.Config{
		// For MongoDB Atlas, we need to use the system's root CA certificates
		// If that doesn't work, you can try with InsecureSkipVerify: true
		// but that's not recommended for production
		MinVersion: tls.VersionTLS12,
	}

	clientOptions := options.Client().
		ApplyURI(uri).
		SetTLSConfig(tlsConfig).
		SetServerAPIOptions(options.ServerAPI(options.ServerAPIVersion1)).
		SetTimeout(30 * time.Second).
		SetConnectTimeout(30 * time.Second)

	// Connect with longer timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var err error
	Client, err = mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalf("MongoDB connection failed: %v", err)
	}

	// Test connection with longer timeout
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer pingCancel()

	err = Client.Ping(pingCtx, nil)
	if err != nil {
		log.Fatalf("MongoDB ping failed: %v", err)
	}

	log.Println("Connected to MongoDB!")
}

func GetDB() *mongo.Database {
	if Client == nil {
		log.Fatal("MongoDB client not initialized")
	}
	return Client.Database("midnight_trader")
}
