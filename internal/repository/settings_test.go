package repository

import (
	mongo_helpers "BookClubBot/internal/repository/testing"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestNewSettingsRepository(t *testing.T) {
	t.Run("with nil database", func(t *testing.T) {
		repo, err := NewSettingsRepository(nil)
		assert.Error(t, err)
		assert.Equal(t, ErrNilDatabase, err)
		assert.Nil(t, repo)
	})

	t.Run("with valid database", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping integration test")
		}

		db, clear := mongo_helpers.CreateTestMongoDB(t)
		defer clear()

		repo, err := NewSettingsRepository(db)
		assert.NoError(t, err)
		assert.NotNil(t, repo)
	})
}

func TestSettingsRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	setupTest := func() (*SettingsRepository, func()) {
		db, clear := mongo_helpers.CreateTestMongoDB(t)
		repo, err := NewSettingsRepository(db)
		require.NoError(t, err) // Test will fail here if repo creation fails

		cleanFunc := func() {
			cleanSettings(clear, db)
		}
		return repo, cleanFunc
	}

	createContext := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 2*time.Second)
	}

	t.Run("SaveGroupID", func(t *testing.T) {
		t.Run("when group id doesn't exist", func(t *testing.T) {
			repo, cleanup := setupTest()
			defer cleanup()

			ctx, cancel := createContext()
			defer cancel()

			var newGroupId int64 = 9977
			err := repo.SaveGroupID(ctx, newGroupId)
			assert.NoError(t, err)

			// Verify saved correctly
			ctx, cancel = createContext()
			defer cancel()

			groupId, err := repo.GetGroupId(ctx)
			assert.NoError(t, err)
			assert.Equal(t, newGroupId, groupId)
		})

		t.Run("when group id exists", func(t *testing.T) {
			repo, cleanup := setupTest()
			defer cleanup()

			var initialGroupId int64 = 9977

			ctx, cancel := createContext()
			defer cancel()
			err := repo.SaveGroupID(ctx, initialGroupId)
			assert.NoError(t, err)

			var newGroupId int64 = 777777
			ctx, cancel = createContext()
			defer cancel()
			err = repo.SaveGroupID(ctx, newGroupId)
			assert.NoError(t, err)

			// Verify updated correctly
			ctx, cancel = createContext()
			defer cancel()
			groupId, err := repo.GetGroupId(ctx)
			assert.NoError(t, err)
			assert.Equal(t, newGroupId, groupId)
		})
	})

	t.Run("GetGroupId", func(t *testing.T) {
		t.Run("when setting exists", func(t *testing.T) {
			repo, cleanup := setupTest()
			defer cleanup()

			// First save a group ID
			var expectedGroupId int64 = 12345
			ctx, cancel := createContext()
			defer cancel()
			err := repo.SaveGroupID(ctx, expectedGroupId)
			require.NoError(t, err)

			// Then retrieve it
			ctx, cancel = createContext()
			defer cancel()
			groupId, err := repo.GetGroupId(ctx)
			assert.NoError(t, err)
			assert.Equal(t, expectedGroupId, groupId)
		})

		t.Run("when setting doesn't exist", func(t *testing.T) {
			repo, cleanup := setupTest()
			defer cleanup()

			ctx, cancel := createContext()
			defer cancel()
			groupId, err := repo.GetGroupId(ctx)
			assert.Error(t, err)
			assert.True(t, errors.Is(err, mongo.ErrNoDocuments), "Expected mongo.ErrNoDocuments, got: %v", err)
			assert.Equal(t, int64(0), groupId)
		})
	})
}

// cleanSettings cleans up the settings collection after tests
func cleanSettings(clear func(), mongoDB *mongo.Database) {
	clear()
	if mongoDB != nil {
		mongo_helpers.DropCollection(mongoDB, settings_collection)
	}
}
