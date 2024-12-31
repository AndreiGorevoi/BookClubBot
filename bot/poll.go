package bot

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	started = iota
	bookAsked
	authorAsked
	descriptionAsked
	imageAsked
	finished

	defaultImagePath = "assets/book-with-question-mark.jpg"
)

type BookGathering struct {
	Participants []*Participant
	IsStarted    bool
}

type Participant struct {
	Id        int64
	FirstName string
	LastName  string
	Nick      string
	Status    int
	Book      *Book
}

type Book struct {
	Title       string
	Author      string
	Description string
	PhotoId     string
}

func (p *Participant) bookCaption() string {
	return fmt.Sprintf(
		"📚 *Название*: %s\n👤 *Автор*: %s\n📝 *Описание*: %s",
		p.Book.Title,
		p.Book.Author,
		p.Book.Description,
	)
}

func (p *Participant) bookImage() tgbotapi.InputMediaPhoto {
	if p.Book.PhotoId != "" {
		return tgbotapi.NewInputMediaPhoto(tgbotapi.FileID(p.Book.PhotoId))
	}
	return tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath(defaultImagePath))
}
