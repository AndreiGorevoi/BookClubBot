package bot

import (
	"BookClubBot/internal/models"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const defaultImagePath = "assets/book-with-question-mark.jpg"

// participant is a view model used only to render Telegram messages from a
// stored session participant. The authoritative state lives in MongoDB
// (models.Participant); this type exists so the message-building helpers stay
// decoupled from the persistence model.
type participant struct {
	id        int64
	firstName string
	lastName  string
	nick      string
	book      *book
}

type book struct {
	title       string
	author      string
	description string
	photoId     string
}

// viewParticipant converts a persisted participant into a render-only view.
func viewParticipant(p *models.Participant) *participant {
	vp := &participant{
		id:        p.SubscriberID,
		firstName: p.FirstName,
		lastName:  p.LastName,
		nick:      p.Nick,
	}
	if p.Book != nil {
		vp.book = &book{
			title:       p.Book.Title,
			author:      p.Book.Author,
			description: p.Book.Description,
			photoId:     p.Book.PhotoID,
		}
	}
	return vp
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
