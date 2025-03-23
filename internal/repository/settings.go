package repository

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const settings_collection = "settings"

type SettingsRepository struct {
	db *mongo.Database
}

func NewSettingsRepository(db *mongo.Database) (*SettingsRepository, error) {
	if db == nil {
		return nil, ErrNilDatabase
	}
	return &SettingsRepository{
		db: db,
	}, nil
}

func (s *SettingsRepository) SaveGroupID(ctx context.Context, groupId int64) error {
	collection := s.db.Collection(settings_collection)
	opts := options.Update().SetUpsert(true)
	filter := bson.M{"_id": "settings"}
	update := bson.M{"$set": bson.M{"groupId": groupId}}

	_, err := collection.UpdateOne(ctx, filter, update, opts)
	return err
}

func (s *SettingsRepository) GetGroupId(ctx context.Context) (int64, error) {
	collection := s.db.Collection(settings_collection)
	var res struct {
		GroupId int64 `bson:"groupId"`
	}

	filter := bson.M{"_id": "settings"}
	if err := collection.FindOne(ctx, filter).Decode(&res); err != nil {
		return 0, err
	}

	return res.GroupId, nil
}
