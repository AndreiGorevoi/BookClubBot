package bot

import (
	"math/rand"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// winningOptionIdx returns the positions of the poll options that tied for the
// most votes. A poll nobody voted in has no winner, so it returns nothing rather
// than reporting every option as a winner.
func winningOptionIdx(res *tgbotapi.Poll) []int {
	if res == nil {
		return nil
	}
	max := 0
	for _, o := range res.Options {
		if o.VoterCount > max {
			max = o.VoterCount
		}
	}
	if max == 0 {
		return nil
	}

	idx := make([]int, 0, len(res.Options))
	for i, o := range res.Options {
		if o.VoterCount == max {
			idx = append(idx, i)
		}
	}
	return idx
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
		bookImage.Caption = truncateString(participant.bookCaption(), 1024)
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

func truncateString(input string, limit int) string {
	// Check the rune count in the string
	if utf8.RuneCountInString(input) <= limit {
		return input // Return as-is if within the limit
	}

	// Truncate to the specified limit
	runes := []rune(input)       // Convert string to a slice of runes (characters)
	return string(runes[:limit]) // Take only the first 'limit' runes
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
