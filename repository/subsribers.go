package repository

import (
	"database/sql"
	"errors"
	"fmt"
)

var ErrUserAlreadySubscribed = errors.New("user with this ID is already subscribed")

type Subscriber struct {
	Id        int64  `db:"id"`
	FirstName string `db:"first_name"`
	LastName  string `db:"last_name"`
	Nick      string `db:"nick"`
	Archived  bool   `db:"archived"`
}

type SubscriberRepository struct {
	db *sql.DB
}

func NewSubscriberRepository(db *sql.DB) *SubscriberRepository {
	return &SubscriberRepository{
		db: db,
	}
}

func (r *SubscriberRepository) GetAll() ([]Subscriber, error) {
	q := "SELECT id, first_name, last_name, nick, archived FROM subscriber where archived = false"
	rows, err := r.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subscribers []Subscriber
	var sub Subscriber
	for rows.Next() {
		if err := rows.Scan(&sub.Id, &sub.FirstName, &sub.LastName, &sub.Nick, &sub.Archived); err != nil {
			return nil, err
		}
		subscribers = append(subscribers, sub)
	}
	return subscribers, nil
}

func (r *SubscriberRepository) AddSubscriber(sub Subscriber) error {
	q := "INSERT or IGNORE INTO subscriber(id, first_name, last_name, nick) VALUES(?, ?, ?, ?)"
	res, err := r.db.Exec(q, sub.Id, sub.FirstName, sub.LastName, sub.Nick)
	if err != nil {
		return err
	}

	affectedRows, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if affectedRows == 0 {
		return ErrUserAlreadySubscribed
	}

	return nil
}

func (r *SubscriberRepository) ArchivedSubscriber(id int64) error {
	q := "UPDATE subscriber SET archived = true where id = ?"
	result, err := r.db.Exec(q, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no subscriber found with id %d", id)
	}

	return nil
}

func (r *SubscriberRepository) FindById(id int64) (*Subscriber, error) {
	q := "SELECT id, first_name, last_name, nick, archived FROM subscriber where id = ? and archived = false"
	var s Subscriber
	err := r.db.QueryRow(q, id).Scan(&s.Id, &s.FirstName, &s.LastName, &s.Nick, &s.Archived)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &s, nil
}
