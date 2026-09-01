package bot

import (
	"strings"
	"testing"

	"BookClubBot/internal/models"
	"BookClubBot/message"

	"github.com/stretchr/testify/assert"
)

func TestBookCaptionEscapesMarkdown(t *testing.T) {
	// The realistic trigger: a description pasted in from a web page, bringing
	// footnote markers and stray emphasis characters with it.
	p := &participant{book: &book{
		title:       "Война и мир_том 1",
		author:      "Лев *Толстой*",
		description: "Роман-эпопея[1] о войне 1812 года[2]. Смотри `главу 3`.",
	}}

	assert.Equal(t,
		"📚 *Название*: Война и мир\\_том 1\n"+
			"👤 *Автор*: Лев \\*Толстой\\*\n"+
			"📝 *Описание*: Роман-эпопея\\[1] о войне 1812 года\\[2]. Смотри \\`главу 3\\`.",
		p.bookCaption())
}

func TestTruncateCaption(t *testing.T) {
	t.Run("a short caption is untouched", func(t *testing.T) {
		assert.Equal(t, "hello", truncateCaption("hello"))
	})

	t.Run("an over-long caption is cut to the limit", func(t *testing.T) {
		out := truncateCaption(strings.Repeat("я", 2000))
		assert.Equal(t, telegramCaptionMaxLen, utf16Len(out))
	})

	t.Run("emoji count double, as Telegram counts them", func(t *testing.T) {
		// 600 emoji is 600 runes but 1200 UTF-16 units: a rune-based cut would
		// have left this over the limit.
		out := truncateCaption(strings.Repeat("📚", 600))
		assert.LessOrEqual(t, utf16Len(out), telegramCaptionMaxLen)
	})

	t.Run("the cut never leaves a dangling escape", func(t *testing.T) {
		// Escaped text is half backslashes, so the cut lands on one sooner or later.
		escaped := escapeMarkdown(strings.Repeat("_", 2000))
		out := truncateCaption(escaped)

		trailing := 0
		for i := len(out) - 1; i >= 0 && out[i] == '\\'; i-- {
			trailing++
		}
		assert.Equal(t, 0, trailing%2, "caption ends with an unpaired backslash: %q", out[len(out)-4:])
	})
}

func TestReviewSummaryFitsTelegramLimit(t *testing.T) {
	b := &Bot{messages: &message.LocalizedMessages{
		BookReviewSummary: "Проверь:\n\n📚 Название: %s\n👤 Автор: %s\n📝 Описание: %s\n🖼 Обложка: %s",
	}}

	t.Run("a normal submission is left alone", func(t *testing.T) {
		p := &models.Participant{Book: &models.Book{Title: "Дюна", Author: "Герберт", Description: "Пески."}}
		out := b.reviewSummary(p, "есть")

		assert.Contains(t, out, "Пески.")
		assert.NotContains(t, out, "…")
	})

	t.Run("a pasted wall of text is trimmed to fit", func(t *testing.T) {
		// Telegram caps an incoming message at 4096, so this is the worst a member
		// can actually send as a description.
		p := &models.Participant{Book: &models.Book{
			Title:       "Дюна",
			Author:      "Фрэнк Герберт",
			Description: strings.Repeat("очень длинное описание ", 200),
		}}
		out := b.reviewSummary(p, "есть")

		assert.LessOrEqual(t, utf16Len(out), telegramMessageMaxLen)
		// The fields around the description survive, so the summary stays useful.
		assert.Contains(t, out, "Дюна")
		assert.Contains(t, out, "Фрэнк Герберт")
		assert.Contains(t, out, "Обложка: есть")
		assert.True(t, strings.Contains(out, "…"), "expected the description to be elided")
	})

	t.Run("a pathological title alone still yields a sendable message", func(t *testing.T) {
		p := &models.Participant{Book: &models.Book{
			Title:       strings.Repeat("я", 5000),
			Author:      "Автор",
			Description: "",
		}}
		out := b.reviewSummary(p, "нет")

		assert.LessOrEqual(t, utf16Len(out), telegramMessageMaxLen)
	})
}
