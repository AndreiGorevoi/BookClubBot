package bot

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

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
		bookImage.Caption = participant.bookCaption()
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
