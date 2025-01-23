package bot

import (
	"unicode/utf8"

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
