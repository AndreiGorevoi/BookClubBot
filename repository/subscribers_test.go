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

		mock.ExpectQuery(`SELECT id, first_name, last_name, nick, archived FROM subscriber where id = \?`).
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

		mock.ExpectQuery(`SELECT id, first_name, last_name, nick, archived FROM subscriber where id = \?`).
			WithArgs(77).
			WillReturnRows(sqlmock.NewRows([]string{"id", "first_name", "last_name", "nick", "archived"}))

		sub, err := repo.FindById(77)
		assert.NoError(t, err)
		assert.Nil(t, sub)
	})

	t.Run("Db returns error", func(t *testing.T) {
		repo, mock := setupSubscriberMockDB(t)

		mock.ExpectQuery(`SELECT id, first_name, last_name, nick, archived FROM subscriber where id = \?`).
			WithArgs(77).
			WillReturnError(errors.New("db error"))

		sub, err := repo.FindById(77)
		assert.Error(t, err)
		assert.Nil(t, sub)
	})
}

func TestSetSubscriberArchived(t *testing.T) {
	t.Run("Successfully archives subscriber", func(t *testing.T) {
		repo, mock := setupSubscriberMockDB(t)

		mock.ExpectExec(`UPDATE subscriber SET archived = \? where id = \?`).
			WithArgs(true, 77).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.SetSubscriberArchived(77, true)
		assert.NoError(t, err)
	})

	t.Run("Successfully unarchives subscriber", func(t *testing.T) {
		repo, mock := setupSubscriberMockDB(t)

		mock.ExpectExec(`UPDATE subscriber SET archived = \? where id = \?`).
			WithArgs(false, 77).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.SetSubscriberArchived(77, false)
		assert.NoError(t, err)
	})

	t.Run("No subscriber found", func(t *testing.T) {
		repo, mock := setupSubscriberMockDB(t)

		mock.ExpectExec(`UPDATE subscriber SET archived = \? where id = \?`).
			WithArgs(true, 99).
			WillReturnResult(sqlmock.NewResult(1, 0))

		err := repo.SetSubscriberArchived(99, true)
		assert.Error(t, err)
		assert.Equal(t, "no subscriber found with id 99", err.Error())
	})

	t.Run("Database returns an error", func(t *testing.T) {
		repo, mock := setupSubscriberMockDB(t)

		mock.ExpectExec(`UPDATE subscriber SET archived = \? where id = \?`).
			WithArgs(true, 77).
			WillReturnError(errors.New("db error"))

		err := repo.SetSubscriberArchived(77, true)
		assert.Error(t, err)
		assert.Equal(t, "db error", err.Error())
	})
}
