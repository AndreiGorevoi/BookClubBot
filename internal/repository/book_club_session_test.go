package repository

import (
	"BookClubBot/internal/models"
	mongo_helpers "BookClubBot/internal/repository/testing"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestSaveBookClubSession(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	t.Run("Save New Book Club Session", func(t *testing.T) {
		mongoDb, clear := mongo_helpers.CreateTestMongoDB(t)
		defer clear()

		repo := createBookClubSessionRepository(mongoDb, t)

		newBookClubSession := &models.BookClubSession{
			Name:   "June 2024",
			Date:   time.Now(),
			Active: true,
			TelegramPoll: models.TelegramPoll{
				ID:         "someId",
				Active:     true,
				Voted:      0,
				TotalVotes: 8,
			},
			BookGathering: models.BookGathering{
				Active:         true,
				BooksSuggested: 0,
				TotalBooks:     8,
			},
			Participants: []models.Participant{
				{ID: 123, NickName: "Jonh98", Status: 1, Book: models.Book{Name: "Book1", Author: "Author1", Image: "1234sswf2"}},
			},
		}
		saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer saveCancel()
		err := repo.Create(saveCtx, newBookClubSession)
		assert.NoError(t, err)

		findCtx, findCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer findCancel()
		foundBookClubSession, err := repo.GetById(findCtx, newBookClubSession.ID)
		assert.NoError(t, err)
		assert.NotNil(t, foundBookClubSession)
		assert.Equal(t, foundBookClubSession.Name, newBookClubSession.Name)
	})

	t.Run("Find Active Session", func(t *testing.T) {
		mongoDb, clear := mongo_helpers.CreateTestMongoDB(t)
		defer clear()

		repo := createBookClubSessionRepository(mongoDb, t)

		newBookClubSession := &models.BookClubSession{
			Name:   "June 2024",
			Date:   time.Now(),
			Active: true,
			TelegramPoll: models.TelegramPoll{
				ID:         "someId",
				Active:     true,
				Voted:      0,
				TotalVotes: 8,
			},
			BookGathering: models.BookGathering{
				Active:         true,
				BooksSuggested: 0,
				TotalBooks:     8,
			},
			Participants: []models.Participant{
				{ID: 123, NickName: "Jonh98", Status: 1, Book: models.Book{Name: "Book1", Author: "Author1", Image: "1234sswf2"}},
			},
		}

		saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer saveCancel()
		err := repo.Create(saveCtx, newBookClubSession)
		assert.NoError(t, err)

		findCtx, findCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer findCancel()
		res, err := repo.GetActiveSession(findCtx)
		assert.NotNil(t, res)
		assert.NoError(t, err)
	})

	t.Run("No Active Session", func(t *testing.T) {
		mongoDb, clear := mongo_helpers.CreateTestMongoDB(t)
		defer clear()

		repo := createBookClubSessionRepository(mongoDb, t)

		newBookClubSession := &models.BookClubSession{
			Name:   "June 2024",
			Date:   time.Now(),
			Active: false,
			TelegramPoll: models.TelegramPoll{
				ID:         "someId",
				Active:     true,
				Voted:      0,
				TotalVotes: 8,
			},
			BookGathering: models.BookGathering{
				Active:         true,
				BooksSuggested: 0,
				TotalBooks:     8,
			},
			Participants: []models.Participant{
				{ID: 123, NickName: "Jonh98", Status: 1, Book: models.Book{Name: "Book1", Author: "Author1", Image: "1234sswf2"}},
			},
		}

		newBookClubSession2 := &models.BookClubSession{
			Name:   "July 2024",
			Date:   time.Now(),
			Active: false,
			TelegramPoll: models.TelegramPoll{
				ID:         "someId",
				Active:     true,
				Voted:      0,
				TotalVotes: 8,
			},
			BookGathering: models.BookGathering{
				Active:         true,
				BooksSuggested: 0,
				TotalBooks:     8,
			},
			Participants: []models.Participant{
				{ID: 123, NickName: "Jonh98", Status: 1, Book: models.Book{Name: "Book1", Author: "Author1", Image: "1234sswf2"}},
			},
		}

		saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer saveCancel()
		err := repo.Create(saveCtx, newBookClubSession)
		assert.NoError(t, err)

		saveCtx2, saveCancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer saveCancel2()
		err = repo.Create(saveCtx2, newBookClubSession2)
		assert.NoError(t, err)

		findCtx, findCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer findCancel()
		res, err := repo.GetActiveSession(findCtx)
		assert.Nil(t, res)
		assert.NoError(t, err)
	})
}

func createBookClubSessionRepository(db *mongo.Database, t *testing.T) *BookClubSessionRepository {
	repo, err := NewBookClubSessionRepository(db)
	assert.NoError(t, err)
	assert.NotNil(t, repo)
	return repo
}
