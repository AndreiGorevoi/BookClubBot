package testing

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const mongo_uri = "mongodb://localhost:27017"

func CreateTestMongoDB(t *testing.T) (*mongo.Database, func()) {
	// Context for initial connection
	connectionCtx, connectionCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer connectionCancel() // Cancel after connection is established

	client, err := mongo.Connect(connectionCtx, options.Client().ApplyURI(mongo_uri))
	assert.NoError(t, err)

	// Add an explicit ping to verify connection is valid
	err = client.Ping(connectionCtx, nil)
	if err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	testDB := client.Database("test_db")

	return testDB, func() {
		// Use a fresh context for disconnection
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()

		err := testDB.Drop(disconnectCtx)
		if err != nil {
			t.Logf("Error dropping test database: %v", err)
		}

		err = client.Disconnect(disconnectCtx)
		if err != nil {
			t.Logf("Error disconnecting from MongoDB: %v", err)
		}
	}
}

func DropCollection(db *mongo.Database, name string) {
	dropCtx, dropCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer dropCancel()
	db.Collection(name).Drop(dropCtx)
}

func TestMongoDBConnection() {
	// Try to connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongo_uri))

	if err != nil || client.Ping(ctx, nil) != nil {
		fmt.Println("MongoDB is not available")
		cancel()
		// Exit with success code - this prevents running the tests but doesn't fail CI
		os.Exit(1)
	}

	// Disconnect and run tests
	client.Disconnect(ctx)
	cancel()
}
