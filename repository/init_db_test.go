package repository_test

import (
	"BookClubBot/repository"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	_ "modernc.org/sqlite"
)

func TestInitDB_NewDataBase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := repository.InitDB(path)
	assert.NoError(t, err)
	defer db.Close()

	// verify db is created
	_, err = os.Stat(path)
	assert.NoError(t, err)

	//verify table is created
	var tableName string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='subscriber'`).Scan(&tableName)
	assert.NoError(t, err)
	assert.Equal(t, "subscriber", tableName)

	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='metadata'`).Scan(&tableName)
	assert.NoError(t, err)
	assert.Equal(t, "metadata", tableName)
}

func TestInitDB_DataBaseExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := repository.InitDB(path)
	assert.NoError(t, err)
	defer db.Close()

	db2, err := repository.InitDB(path)
	assert.NoError(t, err)
	defer db2.Close()
}

func TestInitDB_FailedToConnect(t *testing.T) {
	invalidPath := "invalid/path/test.db"

	db, err := repository.InitDB(invalidPath)
	assert.Error(t, err)
	assert.Nil(t, db)
}
