package bot

import (
	"strings"
	"testing"

	"BookClubBot/internal/models"
	"BookClubBot/message"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePageCount(t *testing.T) {
	valid := map[string]int{
		"320":    320,
		" 320\n": 320,
		"1":      1,
		"10000":  10000,
	}
	for in, want := range valid {
		got, ok := parsePageCount(in)
		assert.True(t, ok, "%q should be accepted", in)
		assert.Equal(t, want, got, "%q", in)
	}

	for _, in := range []string{"", "0", "-5", "10001", "~300", "300 стр", "триста", "3.5", "1e3"} {
		_, ok := parsePageCount(in)
		assert.False(t, ok, "%q should be rejected", in)
	}
}

func pagesFlowBot(tg *fakeTelegram) *Bot {
	return &Bot{
		tgBot: tg.api(),
		messages: &message.LocalizedMessages{
			AskPageCount:     "how many pages?",
			InvalidPageCount: "not a page count",
			AttachCoverPhoto: "attach a cover",
			BtnNoCover:       "No cover",
		},
		sessionRepository: &fakeSessionRepo{},
	}
}

func textFrom(uid int64, text string) *tgbotapi.Update {
	return &tgbotapi.Update{Message: &tgbotapi.Message{
		Text: text,
		From: &tgbotapi.User{ID: uid},
		Chat: &tgbotapi.Chat{ID: uid},
	}}
}

func TestPageCountStep(t *testing.T) {
	tg := newFakeTelegram(t)
	b := pagesFlowBot(tg)
	p := &models.Participant{SubscriberID: 7, Step: models.StepDescription,
		Book: &models.Book{Title: "Dune", Author: "Herbert"}}
	session := sessionWith(p)

	// The description is followed by the page-count question, not the cover.
	b.handleParticipantAnswer(session, p, textFrom(7, "Sand."))
	assert.Equal(t, models.StepPages, p.Step)
	assert.Equal(t, "Sand.", p.Book.Description)

	// Invalid input keeps the member on the step and asks again.
	b.handleParticipantAnswer(session, p, textFrom(7, "about 400"))
	assert.Equal(t, models.StepPages, p.Step)
	assert.Zero(t, p.Book.Pages)

	// A photo sent too early carries no text and is rejected the same way.
	photo := textFrom(7, "")
	photo.Message.Photo = []tgbotapi.PhotoSize{{FileID: "f"}}
	b.handleParticipantAnswer(session, p, photo)
	assert.Equal(t, models.StepPages, p.Step)
	assert.Empty(t, p.Book.PhotoID)

	// A valid number is stored and moves on to the cover.
	b.handleParticipantAnswer(session, p, textFrom(7, " 412 "))
	assert.Equal(t, models.StepImage, p.Step)
	assert.Equal(t, 412, p.Book.Pages)

	var texts []string
	for _, m := range tg.messages() {
		texts = append(texts, m.text)
	}
	require.Equal(t, []string{"how many pages?", "not a page count", "not a page count", "attach a cover"}, texts)
}

func TestPollOptionShowsPages(t *testing.T) {
	b := testBot()
	b.messages.PagesShort = "%d p."

	t.Run("known page count is appended", func(t *testing.T) {
		opt := b.pollOptionFor(&models.Book{Title: "Dune", Author: "Herbert", Pages: 412})
		assert.Equal(t, "Book: Dune. Author: Herbert (412 p.)\n", opt)
	})

	t.Run("unknown page count is left out", func(t *testing.T) {
		opt := b.pollOptionFor(&models.Book{Title: "Dune", Author: "Herbert"})
		assert.Equal(t, "Book: Dune. Author: Herbert\n", opt)
	})

	t.Run("a long option cuts the title and keeps the page count", func(t *testing.T) {
		opt := b.pollOptionFor(&models.Book{
			Title:  strings.Repeat("📚 Очень длинное название ", 10),
			Author: "Herbert",
			Pages:  412,
		})
		assert.LessOrEqual(t, utf16Len(opt), pollOptionMaxLen)
		assert.True(t, strings.HasSuffix(opt, "… (412 p.)"), "got %q", opt)
	})
}

func TestBookCaptionShowsPages(t *testing.T) {
	p := &participant{book: &book{title: "Дюна", author: "Герберт", description: "Пески.", pages: 412}}
	assert.Equal(t, "📚 *Название*: Дюна\n👤 *Автор*: Герберт\n📄 *Страниц*: 412\n📝 *Описание*: Пески.", p.bookCaption())

	t.Run("long fields still fit with the page line", func(t *testing.T) {
		p := &participant{book: &book{title: "Дюна", author: "Герберт",
			description: strings.Repeat("пески ", 400), pages: 412}}
		out := p.bookCaption()
		assert.LessOrEqual(t, utf16Len(out), telegramCaptionMaxLen)
		assert.Contains(t, out, "📄 *Страниц*: 412\n")
	})
}

func TestReviewSummaryShowsPages(t *testing.T) {
	b := &Bot{messages: &message.LocalizedMessages{
		BookReviewSummary: "%s|%s|%s|%s|%s",
		PagesUnknown:      "?",
	}}

	known := &models.Participant{Book: &models.Book{Title: "T", Author: "A", Description: "D", Pages: 412}}
	assert.Equal(t, "T|A|D|412|c", b.reviewSummary(known, "c"))

	// A member mid-flow when the step shipped reaches review without a count.
	legacy := &models.Participant{Book: &models.Book{Title: "T", Author: "A", Description: "D"}}
	assert.Equal(t, "T|A|D|?|c", b.reviewSummary(legacy, "c"))
}
