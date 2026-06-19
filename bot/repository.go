package bot

import (
	"BookClubBot/internal/models"
	"context"
)

type subscriberRepo interface {
	SaveSubscriber(ctx context.Context, subscriber *models.Subscriber) error
	SetArchiveSubscriber(ctx context.Context, subscriberID int64, archived bool) error
	GetAllSubscribers(ctx context.Context) ([]*models.Subscriber, error)
	GetSubscriberById(ctx context.Context, id int64) (*models.Subscriber, error)
}

type settingsRepo interface {
	SaveGroupID(ctx context.Context, groupId int64) error
	GetGroupId(ctx context.Context) (int64, error)
}
