package repository_test

import (
	"BookClubBot/repository"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func setupSubscriberMockDB(t *testing.T) (*repository.SubscriberRepository, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	repo := repository.NewSubscriberRepository(db)
	t.Cleanup(func() { db.Close() })

	return repo, mock
}

func TestFindById(t *testing.T) {
	t.Run("User is found", func(t *testing.T) {
		repo, mock := setupSubscriberMockDB(t)

		mock.ExpectQuery(`SELECT id, first_name, last_name, nick, archived FROM subscriber where id = \? and archived = false`).
			WithArgs(77).
			WillReturnRows(sqlmock.NewRows([]string{"id", "first_name", "last_name", "nick", "archived"}).
				AddRow(77, "John", "Jobs", "jb", false))

		sub, err := repo.FindById(77)
		assert.NoError(t, err)
		assert.NotNil(t, sub)

		// ✅ Check that all values were correctly mapped
		assert.Equal(t, int64(77), sub.Id)
		assert.Equal(t, "John", sub.FirstName)
		assert.Equal(t, "Jobs", sub.LastName)
		assert.Equal(t, "jb", sub.Nick)
		assert.False(t, sub.Archived)
	})

	t.Run("User not found", func(t *testing.T) {
		repo, mock := setupSubscriberMockDB(t)

		mock.ExpectQuery(`SELECT id, first_name, last_name, nick, archived FROM subscriber where id = \? and archived = false`).
			WithArgs(77).
			WillReturnRows(sqlmock.NewRows([]string{"id", "first_name", "last_name", "nick", "archived"}))

		sub, err := repo.FindById(77)
		assert.NoError(t, err)
		assert.Nil(t, sub)
	})

	t.Run("Db returns error", func(t *testing.T) {
		repo, mock := setupSubscriberMockDB(t)

		mock.ExpectQuery(`SELECT id, first_name, last_name, nick, archived FROM subscriber where id = \? and archived = false`).
			WithArgs(77).
			WillReturnError(errors.New("db error"))

		sub, err := repo.FindById(77)
		assert.Error(t, err)
		assert.Nil(t, sub)
	})
}
