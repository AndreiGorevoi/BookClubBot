package repository

import (
	"BookClubBot/internal/models"
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const subs_collection = "subscribers"

type SubscriberRepository struct {
	db *mongo.Database
}

func NewSubscriberRepository(db *mongo.Database) (*SubscriberRepository, error) {
	if db == nil {
		return nil, ErrNilDatabase
	}
	return &SubscriberRepository{
		db: db,
	}, nil
}

func (s *SubscriberRepository) SaveSubscriber(ctx context.Context, subscriber *models.Subscriber) error {
	collection := s.db.Collection(subs_collection)
	opt := options.Update().SetUpsert(true)

	filter := bson.M{"_id": subscriber.ID}
	update := bson.M{"$set": subscriber}

	_, err := collection.UpdateOne(ctx, filter, update, opt)
	return err
}

func (s *SubscriberRepository) ArchiveSubscriber(ctx context.Context, subscriberID int64) error {
	collection := s.db.Collection(subs_collection)
	filter := bson.M{"_id": subscriberID}
	update := bson.M{"$set": bson.M{"archived": true}}

	result, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.ModifiedCount == 0 {
		return fmt.Errorf("subscriber with ID %d not found", subscriberID)
	}

	return nil
}

func (s *SubscriberRepository) GetAllSubscribers(ctx context.Context) ([]*models.Subscriber, error) {
	collection := s.db.Collection(subs_collection)
	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var subscribers []*models.Subscriber

	for cursor.Next(ctx) {
		var subscriber models.Subscriber
		if err := cursor.Decode(&subscriber); err != nil {
			return nil, err
		}
		subscribers = append(subscribers, &subscriber)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return subscribers, nil
}

func (s *SubscriberRepository) GetSubscriberById(ctx context.Context, id int64) (*models.Subscriber, error) {
	collection := s.db.Collection(subs_collection)
	filter := bson.M{"_id": id}
	var subscriber models.Subscriber
	if err := collection.FindOne(ctx, filter).Decode(&subscriber); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("subscriber with id %d not found", id)
		}
		return nil, err
	}

	return &subscriber, nil
}
