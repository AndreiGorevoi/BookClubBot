package repository

import (
	"database/sql"
	"fmt"
	"strconv"
)

type Metadata struct {
	Key   string `db:"keyName"`
	Value string `db:"value"`
}

type MetadataRepository struct {
	db *sql.DB
}

func NewMetadataRepository(db *sql.DB) *MetadataRepository {
	return &MetadataRepository{
		db: db,
	}
}

func (m *MetadataRepository) GetGroupId() (int, error) {
	var value string
	q := "SELECT value FROM metadata where keyName='groupId'"
	err := m.db.QueryRow(q).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("groupId not found in metadata table: %w", err)
		}
		return 0, fmt.Errorf("cannot extract chat id from db for key 'groupId': %w", err)
	}

	res, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("cannot convert value %s to int", value)
	}
	return res, nil
}

func (m *MetadataRepository) SaveGroupId(id int) error {
	q := `
    INSERT INTO metadata(keyName, value)
    VALUES('groupId', ?)
    ON CONFLICT(keyName) DO UPDATE SET value = EXCLUDED.value
    `
	res, err := m.db.Exec(q, strconv.Itoa(id))
	if err != nil {
		return fmt.Errorf("cannot insert groupId '%d' into metadata table: %w", id, err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cannot get affected rows after inserting groupId '%d' into metadata table: %w", id, err)
	}

	if affected == 0 {
		return fmt.Errorf("no rows were affected after attempting to insert groupId '%d' into metadata table", id)
	}

	return nil
}
