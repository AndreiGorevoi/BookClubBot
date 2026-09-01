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

func TestDropDanglingEscape(t *testing.T) {
	assert.Equal(t, "ab", dropDanglingEscape(`ab\`), "an unpaired backslash must go")
	assert.Equal(t, `ab\\`, dropDanglingEscape(`ab\\`), "an escaped backslash must survive")
	assert.Equal(t, `ab\\\`[:4], dropDanglingEscape(`ab\\\`), "three backslashes: one is unpaired")
	assert.Equal(t, "ab", dropDanglingEscape("ab"))
	assert.Equal(t, "", dropDanglingEscape(""))
}

func TestElideEscapedNeverEndsOnAnUnpairedEscape(t *testing.T) {
	// Sweep the cut across escape-sequence boundaries: with a prefix of even and
	// odd length in turn, some of these limits land between a backslash and the
	// character it escapes, which is what the parity guard exists for.
	for _, prefix := range []string{"", "x", "xy"} {
		escaped := escapeMarkdown(prefix + strings.Repeat("_", 600))
		for limit := 200; limit < 240; limit++ {
			out := elideEscaped(escaped, limit)

			assert.LessOrEqual(t, utf16Len(out), limit, "limit %d overshot", limit)
			trailing := 0
			for i := len(out) - len("…") - 1; i >= 0 && out[i] == '\\'; i-- {
				trailing++
			}
			assert.Equal(t, 0, trailing%2, "prefix %q limit %d left an unpaired escape: %q", prefix, limit, out)
		}
	}
}

func TestBookCaptionStaysWithinTelegramLimit(t *testing.T) {
	labels := []string{"*Название*", "*Автор*", "*Описание*"}

	t.Run("a long title cannot cut the template's own markup", func(t *testing.T) {
		// Nothing bounds a title: StepBook stores the message verbatim. Lengths
		// around 1000 are the window where a cut of the assembled caption would
		// land inside the "*Автор*" or "*Описание*" label.
		for _, n := range []int{978, 982, 986, 999, 1002, 1004, 1500} {
			p := &participant{book: &book{
				title:       strings.Repeat("я", n),
				author:      "Толстой",
				description: "Описание.",
			}}
			caption := p.bookCaption()

			assert.LessOrEqual(t, utf16Len(caption), telegramCaptionMaxLen, "title of %d", n)
			for _, label := range labels {
				assert.Contains(t, caption, label, "title of %d cut the %s label", n, label)
			}
		}
	})

	t.Run("a long description is elided, the rest survives", func(t *testing.T) {
		p := &participant{book: &book{
			title:       "Дюна",
			author:      "Фрэнк Герберт",
			description: strings.Repeat("очень длинное описание ", 100),
		}}
		caption := p.bookCaption()

		assert.LessOrEqual(t, utf16Len(caption), telegramCaptionMaxLen)
		assert.Contains(t, caption, "Дюна")
		assert.Contains(t, caption, "Фрэнк Герберт")
		assert.Contains(t, caption, "…", "a trimmed caption should say so")
	})

	t.Run("emoji count double, as Telegram counts them", func(t *testing.T) {
		p := &participant{book: &book{
			title:       strings.Repeat("📚", 600),
			author:      "Автор",
			description: "Описание.",
		}}

		assert.LessOrEqual(t, utf16Len(p.bookCaption()), telegramCaptionMaxLen)
	})

	t.Run("a short caption is left exactly as it was", func(t *testing.T) {
		p := &participant{book: &book{title: "Дюна", author: "Герберт", description: "Пески."}}

		assert.Equal(t, "📚 *Название*: Дюна\n👤 *Автор*: Герберт\n📝 *Описание*: Пески.", p.bookCaption())
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

	t.Run("a long title does not cost the description or the cover line", func(t *testing.T) {
		// The title is what overflows here, so the description — 15 characters —
		// must not be the field that pays for it.
		p := &models.Participant{Book: &models.Book{
			Title:       strings.Repeat("я", 4000),
			Author:      "Герберт",
			Description: "Пески Арракиса.",
		}}
		out := b.reviewSummary(p, "есть")

		assert.LessOrEqual(t, utf16Len(out), telegramMessageMaxLen)
		assert.Contains(t, out, "Пески Арракиса.", "the short description was sacrificed to a long title")
		assert.Contains(t, out, "Обложка: есть", "the cover line was cut off the end")
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
