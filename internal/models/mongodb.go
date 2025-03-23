package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Subscriber представляет участника книжного клуба
type Subscriber struct {
	ID        int64     `bson:"_id"` // Telegram ID как первичный ключ
	FirstName string    `bson:"firstName"`
	LastName  string    `bson:"lastName"`
	Nick      string    `bson:"nick"`
	Archived  bool      `bson:"archived"`
	JoinedAt  time.Time `bson:"joinedAt"`
}

// BookSuggestion представляет предложение книги от участника
type BookSuggestion struct {
	SubscriberID int64     `bson:"subscriberId"`
	BookTitle    string    `bson:"bookTitle"`
	Author       string    `bson:"author"`
	Description  string    `bson:"description"`
	PhotoID      string    `bson:"photoId"`
	SuggestedAt  time.Time `bson:"suggestedAt"`
}

// Voting представляет информацию о голосовании
type Voting struct {
	TelegramPollID    string     `bson:"telegramPollId"`
	StartedAt         time.Time  `bson:"startedAt"`
	CompletedAt       *time.Time `bson:"completedAt"`
	ParticipantsVoted int        `bson:"participantsVoted"`
	TotalParticipants int        `bson:"totalParticipants"`
}

// Winner представляет информацию о книге-победителе
type Winner struct {
	BookTitle    string `bson:"bookTitle"`
	Author       string `bson:"author"`
	Description  string `bson:"description"`
	PhotoID      string `bson:"photoId"`
	SubscriberID int64  `bson:"subscriberId"`
}

// BookClubSession представляет сессию книжного клуба
type BookClubSession struct {
	ID              primitive.ObjectID `bson:"_id,omitempty"`
	Name            string             `bson:"name"`
	Status          string             `bson:"status"` // collecting, voting, completed
	StartedAt       time.Time          `bson:"startedAt"`
	CreatedBy       int64              `bson:"createdBy"`
	BookSuggestions []BookSuggestion   `bson:"bookSuggestions"`
	Voting          Voting             `bson:"voting"`
	Winner          Winner             `bson:"winner"`
}
