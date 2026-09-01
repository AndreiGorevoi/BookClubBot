package bot

import (
	"math/rand"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func defineWinners(res *tgbotapi.Poll) []string {
	if res == nil {
		return nil
	}
	m := make(map[int][]string)
	max := -1

	for _, o := range res.Options {
		if o.VoterCount > max {
			max = o.VoterCount
		}
		m[o.VoterCount] = append(m[o.VoterCount], o.Text)
	}

	return m[max]
}

func splitMedia(participants []*participant, batchSize int) [][]interface{} {
	var batches [][]interface{}
	var currentBatch []interface{}
	for _, participant := range participants {
		// Check if the participant has suggested a book
		if participant.book == nil {
			continue
		}

		// Add an image for the book
		bookImage := participant.bookImage()
		bookImage.Caption = truncateCaption(participant.bookCaption())
		bookImage.ParseMode = "Markdown"
		currentBatch = append(currentBatch, bookImage)
		if len(currentBatch) == batchSize {
			batches = append(batches, currentBatch)
			currentBatch = []interface{}{}
		}
	}

	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}
	return batches
}

// Telegram's length limits, both counted in UTF-16 code units.
const (
	telegramCaptionMaxLen = 1024
	telegramMessageMaxLen = 4096
)

// escapeMarkdown neutralises the legacy-Markdown metacharacters in text the
// members wrote. Without it a single unpaired '_', '*', '`' or '[' — a footnote
// marker pasted in from a web page, say — makes Telegram reject the whole send
// with "can't parse entities". Mirrors escapeHTML in reminders.go.
func escapeMarkdown(s string) string {
	return tgbotapi.EscapeText(tgbotapi.ModeMarkdown, s)
}

// truncateCaption cuts an already-escaped caption to Telegram's caption limit.
// Cutting could leave a trailing lone backslash, which would escape whatever
// follows and break parsing, so an unpaired one is dropped.
func truncateCaption(s string) string {
	if utf16Len(s) <= telegramCaptionMaxLen {
		return s
	}
	out := truncateUTF16(s, telegramCaptionMaxLen)
	trailing := 0
	for i := len(out) - 1; i >= 0 && out[i] == '\\'; i-- {
		trailing++
	}
	if trailing%2 == 1 {
		out = out[:len(out)-1]
	}
	return out
}

// utf16Len reports the length of s the way Telegram measures text: in UTF-16
// code units, so a character outside the BMP (an emoji, say) counts as two.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// truncateUTF16 cuts s down to at most limit UTF-16 code units. It never splits
// a character: one that would straddle the limit is dropped whole.
func truncateUTF16(s string, limit int) string {
	n := 0
	for i, r := range s {
		w := 1
		if r > 0xFFFF {
			w = 2
		}
		if n+w > limit {
			return s[:i]
		}
		n += w
	}
	return s
}

func shuffleSlice[T any](s []T) []T {
	copyS := make([]T, len(s))
	copy(copyS, s)
	rand.Shuffle(len(copyS), func(i, j int) {
		copyS[i], copyS[j] = copyS[j], copyS[i]
	})
	return copyS
}
