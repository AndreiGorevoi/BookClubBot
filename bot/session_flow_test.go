package bot

import (
	"BookClubBot/internal/models"
	"BookClubBot/message"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
)

func testBot() *Bot {
	return &Bot{
		messages: &message.LocalizedMessages{
			BookLabel:   "Book",
			AuthorLabel: "Author",
		},
	}
}

func sessionWith(participants ...*models.Participant) *models.BookClubSession {
	return &models.BookClubSession{
		Gathering: models.Gathering{Participants: participants},
	}
}

func TestFindParticipant(t *testing.T) {
	session := sessionWith(
		&models.Participant{SubscriberID: 1},
		&models.Participant{SubscriberID: 2},
	)

	assert.Equal(t, int64(2), findParticipant(session, 2).SubscriberID)
	assert.Nil(t, findParticipant(session, 99))
}

func TestIsBookAlreadyProposed(t *testing.T) {
	session := sessionWith(
		&models.Participant{SubscriberID: 1, Book: &models.Book{Title: "Dune"}},
		&models.Participant{SubscriberID: 2, Step: models.StepBook}, // no book yet
	)

	assert.True(t, isBookAlreadyProposed(session, "Dune"))
	assert.False(t, isBookAlreadyProposed(session, "Neuromancer"))
}

func TestAllBooksChosen(t *testing.T) {
	t.Run("all done or skipped", func(t *testing.T) {
		session := sessionWith(
			&models.Participant{SubscriberID: 1, Step: models.StepDone},
			&models.Participant{SubscriberID: 2, Step: models.StepSkipped},
		)
		assert.True(t, allBooksChosen(session))
	})

	t.Run("someone still answering", func(t *testing.T) {
		session := sessionWith(
			&models.Participant{SubscriberID: 1, Step: models.StepDone},
			&models.Participant{SubscriberID: 2, Step: models.StepAuthor},
		)
		assert.False(t, allBooksChosen(session))
	})
}

func TestExtractBooks(t *testing.T) {
	b := testBot()
	session := sessionWith(
		&models.Participant{SubscriberID: 1, Step: models.StepDone, Book: &models.Book{Title: "Dune", Author: "Herbert"}},
		&models.Participant{SubscriberID: 2, Step: models.StepSkipped},
		&models.Participant{SubscriberID: 3, Step: models.StepImage, Book: &models.Book{Title: "Partial"}}, // not done
	)

	books := b.extractBooks(session)
	assert.Equal(t, []string{"Book: Dune. Author: Herbert\n"}, books)
}

func TestWinnersFromPoll(t *testing.T) {
	b := testBot()
	dune := &models.Book{Title: "Dune", Author: "Herbert"}
	neuro := &models.Book{Title: "Neuromancer", Author: "Gibson"}
	session := sessionWith(
		&models.Participant{SubscriberID: 1, Step: models.StepDone, Book: dune},
		&models.Participant{SubscriberID: 2, Step: models.StepDone, Book: neuro},
	)

	t.Run("single winner maps to its book", func(t *testing.T) {
		poll := &tgbotapi.Poll{Options: []tgbotapi.PollOption{
			{Text: b.pollOptionFor(dune), VoterCount: 3},
			{Text: b.pollOptionFor(neuro), VoterCount: 1},
		}}
		winners := b.winnersFromPoll(session, poll)
		assert.Equal(t, []models.Winner{{SubscriberID: 1, Title: "Dune", Author: "Herbert"}}, winners)
	})

	t.Run("tie returns both", func(t *testing.T) {
		poll := &tgbotapi.Poll{Options: []tgbotapi.PollOption{
			{Text: b.pollOptionFor(dune), VoterCount: 2},
			{Text: b.pollOptionFor(neuro), VoterCount: 2},
		}}
		winners := b.winnersFromPoll(session, poll)
		assert.Len(t, winners, 2)
		ids := []int64{winners[0].SubscriberID, winners[1].SubscriberID}
		assert.ElementsMatch(t, []int64{1, 2}, ids)
	})

	t.Run("no options yields no winners", func(t *testing.T) {
		winners := b.winnersFromPoll(session, &tgbotapi.Poll{})
		assert.Empty(t, winners)
	})

	t.Run("zero votes yields no winners", func(t *testing.T) {
		poll := &tgbotapi.Poll{Options: []tgbotapi.PollOption{
			{Text: b.pollOptionFor(dune), VoterCount: 0},
			{Text: b.pollOptionFor(neuro), VoterCount: 0},
		}}
		winners := b.winnersFromPoll(session, poll)
		assert.Empty(t, winners)
	})
}
