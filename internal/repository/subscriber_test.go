package repository

import (
	"BookClubBot/internal/models"
	mongo_helpers "BookClubBot/internal/repository/testing"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Test constructor with valid and nil database
func TestNewSubscriberRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Valid database case
	mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
	defer clear()

	repo, err := NewSubscriberRepository(mongoDB)
	assert.NoError(t, err)
	assert.NotNil(t, repo)

	// Nil database case
	repo, err = NewSubscriberRepository(nil)
	assert.Error(t, err)
	assert.Nil(t, repo)
	assert.Equal(t, ErrNilDatabase, err)
}

func TestSaveSubscriber(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
	defer cleanSubscriber(clear, mongoDB)

	repo, err := NewSubscriberRepository(mongoDB)
	assert.NoError(t, err)

	// Current time to test JoinedAt
	now := time.Now().UTC().Truncate(time.Millisecond)

	subscriber := &models.Subscriber{
		ID:        123,
		FirstName: "John",
		LastName:  "Spenser",
		Nick:      "JS",
		JoinedAt:  now,
	}

	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = repo.SaveSubscriber(saveCtx, subscriber)
	assert.NoError(t, err)

	// Add explicit timeout for find operation
	findCtx, findCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer findCancel()
	var saved models.Subscriber
	err = repo.db.Collection(subs_collection).FindOne(findCtx, bson.M{"_id": 123}).Decode(&saved)
	assert.NoError(t, err)
	assert.Equal(t, subscriber.ID, saved.ID)
	assert.Equal(t, subscriber.FirstName, saved.FirstName)
	assert.Equal(t, subscriber.LastName, saved.LastName)
	assert.Equal(t, subscriber.Nick, saved.Nick)
	assert.Equal(t, subscriber.JoinedAt.UTC(), saved.JoinedAt.UTC())
	assert.Equal(t, false, saved.Archived) // Explicitly check default Archived value
}

func TestUpdateSubscriber(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
	defer cleanSubscriber(clear, mongoDB)

	repo, err := NewSubscriberRepository(mongoDB)
	assert.NoError(t, err)

	joinedTime := time.Now().UTC().Truncate(time.Millisecond)
	initialSubscriber := &models.Subscriber{
		ID:        456,
		FirstName: "Jane",
		LastName:  "Doe",
		Nick:      "JD",
		JoinedAt:  joinedTime,
	}

	// Insert the initial subscriber
	insertSubscriber(repo, t, initialSubscriber)

	// Update the subscriber using SaveSubscriber method
	updatedJoinedTime := joinedTime.Add(24 * time.Hour) // One day later
	updatedSubscriber := &models.Subscriber{
		ID:        456, // Same ID
		FirstName: "Jane",
		LastName:  "Smith",           // Changed last name
		Nick:      "JS",              // Changed nickname
		Archived:  true,              // Changed archived status
		JoinedAt:  updatedJoinedTime, // Updated join time
	}

	// Test the SaveSubscriber method for update
	updateCtx, updateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer updateCancel()
	err = repo.SaveSubscriber(updateCtx, updatedSubscriber)
	assert.NoError(t, err)

	// Verify the update
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer verifyCancel()
	var updated models.Subscriber
	err = repo.db.Collection(subs_collection).FindOne(verifyCtx, bson.M{"_id": 456}).Decode(&updated)
	assert.NoError(t, err)

	// Assert that fields were updated
	assert.Equal(t, updatedSubscriber.ID, updated.ID)
	assert.Equal(t, updatedSubscriber.FirstName, updated.FirstName)
	assert.Equal(t, updatedSubscriber.LastName, updated.LastName)
	assert.Equal(t, updatedSubscriber.Nick, updated.Nick)
	assert.Equal(t, updatedSubscriber.Archived, updated.Archived)
	assert.Equal(t, updatedSubscriber.JoinedAt.UTC(), updated.JoinedAt.UTC())

	// Assert that it's the same document (not a new one)
	countCtx, countCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer countCancel()
	count, err := repo.db.Collection(subs_collection).CountDocuments(countCtx, bson.M{"_id": 456})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestArchiveSubscriber(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	t.Run("Archive existing subscriber", func(t *testing.T) {
		mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
		defer cleanSubscriber(clear, mongoDB)

		repo, err := NewSubscriberRepository(mongoDB)
		assert.NoError(t, err)

		joinedTime := time.Now().UTC().Truncate(time.Millisecond)
		initialSubscriber := &models.Subscriber{
			ID:        456,
			FirstName: "Jane",
			LastName:  "Doe",
			Nick:      "JD",
			JoinedAt:  joinedTime,
		}

		// Insert the initial subscriber
		insertSubscriber(repo, t, initialSubscriber)

		archiveCtx, archiveCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer archiveCancel()
		err = repo.ArchiveSubscriber(archiveCtx, 456)
		assert.NoError(t, err)

		// Verify the update
		verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer verifyCancel()
		var updated models.Subscriber
		err = repo.db.Collection(subs_collection).FindOne(verifyCtx, bson.M{"_id": 456}).Decode(&updated)
		assert.NoError(t, err)
		assert.True(t, updated.Archived)
	})

	t.Run("Archive non-existing user", func(t *testing.T) {
		mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
		defer cleanSubscriber(clear, mongoDB)

		repo, err := NewSubscriberRepository(mongoDB)
		assert.NoError(t, err)

		archiveCtx, archiveCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer archiveCancel()
		err = repo.ArchiveSubscriber(archiveCtx, 456)
		assert.Error(t, err)
		errorMsg := fmt.Sprintf("subscriber with ID %d not found", 456)
		assert.ErrorContains(t, err, errorMsg)
	})
}

func TestGetAllSubscribers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
	defer cleanSubscriber(clear, mongoDB)

	repo, err := NewSubscriberRepository(mongoDB)
	assert.NoError(t, err)

	joinedTime := time.Now().UTC().Truncate(time.Millisecond)
	subscribers := []any{
		&models.Subscriber{
			ID:        123,
			FirstName: "Alex",
			LastName:  "Caruso",
			Nick:      "Defenator",
			JoinedAt:  joinedTime,
		},
		&models.Subscriber{
			ID:        234,
			FirstName: "Nick",
			LastName:  "Jepherson",
			Nick:      "Nicky",
			JoinedAt:  joinedTime.Add(1 * time.Hour),
		},
		&models.Subscriber{
			ID:        345,
			FirstName: "Joel",
			LastName:  "Embid",
			Nick:      "Knee",
			JoinedAt:  joinedTime.Add(2 * time.Hour),
		},
	}

	// Insert multiple subscribers
	insertCtx, insertCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer insertCancel()
	result, err := repo.db.Collection(subs_collection).InsertMany(insertCtx, subscribers)
	assert.NoError(t, err)
	assert.Len(t, result.InsertedIDs, 3)

	// Test GetAllSubscribers
	getAllCtx, getAllCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer getAllCancel()
	retrievedSubs, err := repo.GetAllSubscribers(getAllCtx)
	assert.NoError(t, err)
	assert.Len(t, retrievedSubs, 3)

	// Verify each subscriber was retrieved with correct data
	for _, sub := range retrievedSubs {
		assert.Contains(t, []int64{123, 234, 345}, sub.ID)
		switch sub.ID {
		case 123:
			assert.Equal(t, "Alex", sub.FirstName)
			assert.Equal(t, "Caruso", sub.LastName)
			assert.Equal(t, "Defenator", sub.Nick)
			assert.Equal(t, joinedTime.Unix(), sub.JoinedAt.Unix())
		case 234:
			assert.Equal(t, "Nick", sub.FirstName)
			assert.Equal(t, "Jepherson", sub.LastName)
			assert.Equal(t, "Nicky", sub.Nick)
			assert.Equal(t, joinedTime.Add(1*time.Hour).Unix(), sub.JoinedAt.Unix())
		case 345:
			assert.Equal(t, "Joel", sub.FirstName)
			assert.Equal(t, "Embid", sub.LastName)
			assert.Equal(t, "Knee", sub.Nick)
			assert.Equal(t, joinedTime.Add(2*time.Hour).Unix(), sub.JoinedAt.Unix())
		}
	}
}

func TestGetSubscriberById(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	t.Run("User exists", func(t *testing.T) {
		mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
		defer cleanSubscriber(clear, mongoDB)

		repo, err := NewSubscriberRepository(mongoDB)
		assert.NoError(t, err)

		joinedTime := time.Now().UTC().Truncate(time.Millisecond)
		initialSubscriber := &models.Subscriber{
			ID:        456,
			FirstName: "Jane",
			LastName:  "Doe",
			Nick:      "JD",
			JoinedAt:  joinedTime,
		}

		// Insert the initial subscriber
		insertSubscriber(repo, t, initialSubscriber)

		getByIdCtx, getByIdCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer getByIdCancel()
		sub, err := repo.GetSubscriberById(getByIdCtx, 456)
		assert.NoError(t, err)
		assert.Equal(t, initialSubscriber.ID, sub.ID)
		assert.Equal(t, initialSubscriber.FirstName, sub.FirstName)
		assert.Equal(t, initialSubscriber.LastName, sub.LastName)
		assert.Equal(t, initialSubscriber.Nick, sub.Nick)
		assert.Equal(t, initialSubscriber.JoinedAt.UTC(), sub.JoinedAt.UTC())
		assert.False(t, sub.Archived)
	})

	t.Run("User doesn't exist", func(t *testing.T) {
		mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
		defer cleanSubscriber(clear, mongoDB)

		repo, err := NewSubscriberRepository(mongoDB)
		assert.NoError(t, err)

		getByIdCtx, getByIdCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer getByIdCancel()
		sub, err := repo.GetSubscriberById(getByIdCtx, 456)
		assert.Error(t, err)
		errMsg := fmt.Sprintf("subscriber with id %d not found", 456)
		assert.ErrorContains(t, err, errMsg)
		assert.Nil(t, sub)
	})
}

// Helper function to insert a subscriber and verify the insert
func insertSubscriber(repo *SubscriberRepository, t *testing.T, subscriber *models.Subscriber) {
	insertCtx, insertCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer insertCancel()
	_, err := repo.db.Collection(subs_collection).InsertOne(insertCtx, subscriber)
	assert.NoError(t, err)

	// Verify initial insert
	findCtx, findCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer findCancel()
	var saved models.Subscriber
	err = repo.db.Collection(subs_collection).FindOne(findCtx, bson.M{"_id": subscriber.ID}).Decode(&saved)
	assert.NoError(t, err)
	assert.Equal(t, subscriber.ID, saved.ID)
	assert.Equal(t, subscriber.FirstName, saved.FirstName)
	assert.Equal(t, subscriber.LastName, saved.LastName)
	assert.Equal(t, subscriber.Nick, saved.Nick)
	assert.Equal(t, subscriber.JoinedAt.UTC(), saved.JoinedAt.UTC())
}

// Cleanup helper
func cleanSubscriber(clear func(), mongoDB *mongo.Database) {
	clear()
	mongo_helpers.DropCollection(mongoDB, subs_collection)
}
