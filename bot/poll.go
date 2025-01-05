package bot

import (
	"fmt"
	"slices"

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

type bookGathering struct {
	participants []*participant
	active       bool
}

type participant struct {
	id        int64
	firstName string
	lastName  string
	nick      string
	status    int
	book      *book
}

type book struct {
	title       string
	author      string
	description string
	photoId     string
}

func (bg *bookGathering) isParticipant(id int64) bool {
	for _, p := range bg.participants {
		if p.id == id {
			return true
		}
	}
	return false
}

func (bg *bookGathering) removeParticipant(id int64) {
	for i := 0; i < len(bg.participants); i++ {
		if bg.participants[i].id == id {
			bg.participants = slices.Delete(bg.participants, i, i+1)
			return
		}
	}
}

func (p *participant) bookCaption() string {
	return fmt.Sprintf(
		"📚 *Название*: %s\n👤 *Автор*: %s\n📝 *Описание*: %s",
		p.book.title,
		p.book.author,
		p.book.description,
	)
}

func (p *participant) bookImage() tgbotapi.InputMediaPhoto {
	if p.book.photoId != "" {
		return tgbotapi.NewInputMediaPhoto(tgbotapi.FileID(p.book.photoId))
	}
	return tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath(defaultImagePath))
}
