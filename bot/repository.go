package bot

import (
	"BookClubBot/internal/models"
	"context"

	"go.mongodb.org/mongo-driver/bson/primitive"
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

type sessionRepo interface {
	CreateSession(ctx context.Context, session *models.BookClubSession) error
	GetActiveSession(ctx context.Context) (*models.BookClubSession, error)
	UpdateParticipant(ctx context.Context, id primitive.ObjectID, participant *models.Participant) error
	AddVoter(ctx context.Context, id primitive.ObjectID, voterID int64) error
	StartVoting(ctx context.Context, id primitive.ObjectID, voting *models.Voting) error
	SetWinners(ctx context.Context, id primitive.ObjectID, winners []models.Winner) error
	SetStatus(ctx context.Context, id primitive.ObjectID, status string) error
}
