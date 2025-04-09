package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Subscriber struct {
	ID        int64     `bson:"_id"`
	FirstName string    `bson:"firstName"`
	LastName  string    `bson:"lastName"`
	Nick      string    `bson:"nick"`
	Archived  bool      `bson:"archived"`
	JoinedAt  time.Time `bson:"joinedAt"`
}

type BookSuggestion struct {
	SubscriberID int64     `bson:"subscriberId"`
	BookTitle    string    `bson:"bookTitle"`
	Author       string    `bson:"author"`
	Description  string    `bson:"description"`
	PhotoID      string    `bson:"photoId"`
	SuggestedAt  time.Time `bson:"suggestedAt"`
}

type Voting struct {
	TelegramPollID    string     `bson:"telegramPollId"`
	StartedAt         time.Time  `bson:"startedAt"`
	CompletedAt       *time.Time `bson:"completedAt"`
	ParticipantsVoted int        `bson:"participantsVoted"`
	TotalParticipants int        `bson:"totalParticipants"`
}

type Winner struct {
	BookTitle    string `bson:"bookTitle"`
	Author       string `bson:"author"`
	Description  string `bson:"description"`
	PhotoID      string `bson:"photoId"`
	SubscriberID int64  `bson:"subscriberId"`
}

type BookClubSession struct {
	ID              primitive.ObjectID `bson:"_id,omitempty"`
	Name            string             `bson:"name"`
	Status          string             `bson:"status"`
	StartedAt       time.Time          `bson:"startedAt"`
	CreatedBy       int64              `bson:"createdBy"`
	BookSuggestions []BookSuggestion   `bson:"bookSuggestions"`
	Voting          Voting             `bson:"voting"`
	Winner          Winner             `bson:"winner"`
}
