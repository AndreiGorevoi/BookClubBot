package repository

import (
	"database/sql"
	"fmt"
	"os"
)

func InitDB(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		// create db file
		_, err = os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("failed to create database file: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	err = initTables(db)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	return db, nil
}

func initTables(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS subscriber (
        id INTEGER PRIMARY KEY,
        first_name TEXT,
        last_name TEXT,
        nick TEXT,
        archived BOOLEAN DEFAULT false)`)

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS metadata (
		keyName text PRIMARY KEY,
		value TEXT)`)
	return err
}
