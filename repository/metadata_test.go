package repository_test

import (
	"BookClubBot/repository"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestGetGroupId(t *testing.T) {
	t.Run("Success - groupId found", func(t *testing.T) {
		// Mock DB and expectations
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery("SELECT value FROM metadata where keyName='groupId'").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("123"))

		repo := repository.NewMetadataRepository(db)

		// Execute method
		groupId, err := repo.GetGroupId()

		// Assertions
		assert.NoError(t, err)
		assert.Equal(t, 123, groupId)
	})

	t.Run("Error - groupId not found", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery("SELECT value FROM metadata where keyName='groupId'").
			WillReturnError(sql.ErrNoRows)

		repo := repository.NewMetadataRepository(db)

		groupId, err := repo.GetGroupId()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "groupId not found in metadata table")
		assert.Equal(t, 0, groupId)
	})

	t.Run("Error - Conversion failure", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery("SELECT value FROM metadata where keyName='groupId'").
			WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("invalid"))

		repo := repository.NewMetadataRepository(db)

		groupId, err := repo.GetGroupId()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot convert value invalid to int")
		assert.Equal(t, 0, groupId)
	})

	t.Run("Error - Query execution failure", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mock.ExpectQuery("SELECT value FROM metadata where keyName='groupId'").
			WillReturnError(errors.New("query error"))

		repo := repository.NewMetadataRepository(db)

		groupId, err := repo.GetGroupId()

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot extract chat id from db for key 'groupId'")
		assert.Equal(t, 0, groupId)
	})
}

func TestSaveGroupId(t *testing.T) {
	t.Run("Success - groupId saved", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("INSERT INTO metadata\\(keyName, value\\).*").
			WithArgs("123").
			WillReturnResult(sqlmock.NewResult(1, 1))

		repo := repository.NewMetadataRepository(db)

		err = repo.SaveGroupId(123)

		assert.NoError(t, err)
	})

	t.Run("Error - Insert failure", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("INSERT INTO metadata\\(keyName, value\\).*").
			WithArgs("123").
			WillReturnError(errors.New("insert error"))

		repo := repository.NewMetadataRepository(db)

		err = repo.SaveGroupId(123)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot insert groupId '123' into metadata table")
	})

	t.Run("Error - No rows affected", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("INSERT INTO metadata\\(keyName, value\\).*").
			WithArgs("123").
			WillReturnResult(sqlmock.NewResult(1, 0))

		repo := repository.NewMetadataRepository(db)

		err = repo.SaveGroupId(123)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no rows were affected after attempting to insert groupId '123' into metadata table")
	})

	t.Run("Error - RowsAffected failure", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		assert.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("INSERT INTO metadata\\(keyName, value\\).*").
			WithArgs("123").
			WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected error")))

		repo := repository.NewMetadataRepository(db)

		err = repo.SaveGroupId(123)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot get affected rows after inserting groupId '123' into metadata table")
	})
}
