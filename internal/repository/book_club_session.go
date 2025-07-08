package repository

import (
	"BookClubBot/internal/models"
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const book_club_session_collection = "bookClubSessions"

type BookClubSessionRepository struct {
	db *mongo.Database
}

func NewBookClubSessionRepository(db *mongo.Database) (*BookClubSessionRepository, error) {
	if db == nil {
		return nil, ErrNilDatabase
	}

	return &BookClubSessionRepository{
		db: db,
	}, nil
}

func (r *BookClubSessionRepository) Create(ctx context.Context, s *models.BookClubSession) error {
	if !s.ID.IsZero() {
		return fmt.Errorf("new session must not have an ID")
	}
	s.ID = primitive.NewObjectID()
	_, err := r.db.Collection(book_club_session_collection).InsertOne(ctx, s)
	return err
}

func (r *BookClubSessionRepository) Update(ctx context.Context, s *models.BookClubSession) error {
	if s.ID.IsZero() {
		return fmt.Errorf("cannot update session without ID")
	}
	filter := bson.M{"_id": s.ID}
	update := bson.M{"$set": bson.M{
		"name":          s.Name,
		"date":          s.Date,
		"active":        s.Active,
		"telegramPoll":  s.TelegramPoll,
		"bookGathering": s.BookGathering,
		"participants":  s.Participants,
	}}
	res, err := r.db.Collection(book_club_session_collection).UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (bcs *BookClubSessionRepository) GetById(ctx context.Context, id primitive.ObjectID) (*models.BookClubSession, error) {
	collection := bcs.db.Collection(book_club_session_collection)
	filter := bson.M{"_id": id}
	var bookClubSession models.BookClubSession

	if err := collection.FindOne(ctx, filter).Decode(&bookClubSession); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}

	return &bookClubSession, nil
}

func (bcs *BookClubSessionRepository) GetActiveSession(ctx context.Context) (*models.BookClubSession, error) {
	collection := bcs.db.Collection(book_club_session_collection)
	filter := bson.M{"active": true}

	var bookClubSession models.BookClubSession

	if err := collection.FindOne(ctx, filter).Decode(&bookClubSession); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}

	return &bookClubSession, nil
}
