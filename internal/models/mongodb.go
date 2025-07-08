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

// BookClubSession represents a monthly book-club session
type BookClubSession struct {
	ID            primitive.ObjectID `bson:"_id,omitempty"`
	Name          string             `bson:"name"`
	Date          time.Time          `bson:"date"`
	Active        bool               `bson:"active"`
	TelegramPoll  TelegramPoll       `bson:"telegramPoll"`
	BookGathering BookGathering      `bson:"bookGathering"`
	Participants  []Participant      `bson:"participants"`
}

// TelegramPoll holds poll data from Telegram
type TelegramPoll struct {
	ID         string `bson:"id"`
	Active     bool   `bson:"active"`
	Voted      int    `bson:"voted"`
	TotalVotes int    `bson:"totalVotes"`
}

// BookGathering holds info about suggested books
type BookGathering struct {
	Active         bool `bson:"active"`
	BooksSuggested int  `bson:"booksSuggested"`
	TotalBooks     int  `bson:"totalBooks"`
}

// Participant is a user taking part in the session
type Participant struct {
	ID       int64  `bson:"id"`
	NickName string `bson:"nickName"`
	Status   int    `bson:"status"`
	Book     Book   `bson:"book"`
}

// Book describes a suggested or chosen book
type Book struct {
	Name   string `bson:"name"`
	Author string `bson:"author"`
	Image  string `bson:"image"`
}
