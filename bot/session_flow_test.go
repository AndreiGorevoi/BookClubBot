package bot

import (
	"BookClubBot/config"
	"BookClubBot/internal/models"
	"BookClubBot/message"
	"strings"
	"testing"
	"unicode/utf8"

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

// votedPoll is a closed poll in which n options each drew a vote.
func votedPoll(n int) *tgbotapi.Poll {
	poll := &tgbotapi.Poll{}
	for i := 0; i < n; i++ {
		poll.Options = append(poll.Options, tgbotapi.PollOption{VoterCount: 1})
	}
	return poll
}

// emptyPoll is a closed poll with n options that nobody voted in.
func emptyPoll(n int) *tgbotapi.Poll {
	poll := &tgbotapi.Poll{}
	for i := 0; i < n; i++ {
		poll.Options = append(poll.Options, tgbotapi.PollOption{VoterCount: 0})
	}
	return poll
}

// votingOn attaches the poll's option owner list to a session, the way
// runTelegramPoll persists it when the poll is built.
func votingOn(session *models.BookClubSession, owners ...int64) *models.BookClubSession {
	session.Voting = &models.Voting{OptionOwners: owners}
	return session
}

func TestGatheringKeyboards(t *testing.T) {
	b := &Bot{messages: &message.LocalizedMessages{
		BtnSkipGathering: "Skip",
		BtnNoCover:       "No cover",
		BtnConfirmBook:   "Suggest",
		BtnRestartBook:   "Start over",
	}}

	t.Run("skip keyboard carries the skip callback", func(t *testing.T) {
		kb := b.skipKeyboard()
		btn := kb.InlineKeyboard[0][0]
		assert.Equal(t, "Skip", btn.Text)
		assert.NotNil(t, btn.CallbackData)
		assert.Equal(t, callbackSkipGathering, *btn.CallbackData)
	})

	t.Run("no-cover keyboard carries the no-cover callback", func(t *testing.T) {
		kb := b.noCoverKeyboard()
		btn := kb.InlineKeyboard[0][0]
		assert.Equal(t, "No cover", btn.Text)
		assert.NotNil(t, btn.CallbackData)
		assert.Equal(t, callbackNoCover, *btn.CallbackData)
	})

	t.Run("review keyboard carries the confirm and restart callbacks", func(t *testing.T) {
		kb := b.reviewKeyboard()
		confirm := kb.InlineKeyboard[0][0]
		restart := kb.InlineKeyboard[0][1]
		assert.Equal(t, "Suggest", confirm.Text)
		assert.NotNil(t, confirm.CallbackData)
		assert.Equal(t, callbackConfirmBook, *confirm.CallbackData)
		assert.Equal(t, "Start over", restart.Text)
		assert.NotNil(t, restart.CallbackData)
		assert.Equal(t, callbackRestartBook, *restart.CallbackData)
	})
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

	t.Run("someone reviewing their submission", func(t *testing.T) {
		session := sessionWith(
			&models.Participant{SubscriberID: 1, Step: models.StepDone},
			&models.Participant{SubscriberID: 2, Step: models.StepReview},
		)
		assert.False(t, allBooksChosen(session))
	})
}

func TestRunTelegramPollNotEnoughBooks(t *testing.T) {
	b := testBot()
	b.cfg = &config.AppConfig{GroupId: 1}

	// Only one finished book — too few for a poll.
	session := sessionWith(
		&models.Participant{SubscriberID: 1, Step: models.StepDone, Book: &models.Book{Title: "Dune", Author: "Herbert"}},
		&models.Participant{SubscriberID: 2, Step: models.StepSkipped},
	)

	err := b.runTelegramPoll(session)
	assert.ErrorIs(t, err, errNotEnoughBooks)
}

func TestExtractPollOptions(t *testing.T) {
	b := testBot()
	session := sessionWith(
		&models.Participant{SubscriberID: 1, Step: models.StepDone, Book: &models.Book{Title: "Dune", Author: "Herbert"}},
		&models.Participant{SubscriberID: 2, Step: models.StepSkipped},
		&models.Participant{SubscriberID: 3, Step: models.StepImage, Book: &models.Book{Title: "Partial"}}, // not done
	)

	opts := b.extractPollOptions(session)
	assert.Equal(t, []pollOption{{Text: "Book: Dune. Author: Herbert\n", OwnerID: 1}}, opts)
}

func TestWinnersFromPoll(t *testing.T) {
	b := testBot()
	dune := &models.Book{Title: "Dune", Author: "Herbert"}
	neuro := &models.Book{Title: "Neuromancer", Author: "Gibson"}
	session := votingOn(sessionWith(
		&models.Participant{SubscriberID: 1, Step: models.StepDone, Book: dune},
		&models.Participant{SubscriberID: 2, Step: models.StepDone, Book: neuro},
	), 1, 2)

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

func TestPollOptionForTruncatesToTelegramLimit(t *testing.T) {
	b := testBot()

	t.Run("short option is left untouched", func(t *testing.T) {
		opt := b.pollOptionFor(&models.Book{Title: "Dune", Author: "Herbert"})
		assert.Equal(t, "Book: Dune. Author: Herbert\n", opt)
	})

	t.Run("long option is capped and ends with an ellipsis", func(t *testing.T) {
		long := &models.Book{
			Title:  strings.Repeat("Очень длинное название ", 10),
			Author: strings.Repeat("Автор ", 10),
		}
		opt := b.pollOptionFor(long)
		assert.LessOrEqual(t, utf16Len(opt), pollOptionMaxLen)
		assert.True(t, strings.HasSuffix(opt, "…"), "expected an ellipsis, got %q", opt)
	})

	t.Run("cap counts UTF-16 units, as Telegram does", func(t *testing.T) {
		// Every emoji is a surrogate pair: 2 UTF-16 units but 1 rune, so a title
		// well under 100 runes can still exceed Telegram's limit.
		emoji := &models.Book{
			Title:  strings.Repeat("📚", 60),
			Author: "Herbert",
		}
		opt := b.pollOptionFor(emoji)
		assert.Less(t, utf8.RuneCountInString(opt), pollOptionMaxLen, "test setup: the option must be short in runes")
		assert.LessOrEqual(t, utf16Len(opt), pollOptionMaxLen)
		assert.True(t, strings.HasSuffix(opt, "…"), "expected an ellipsis, got %q", opt)
		// The cut must not split a surrogate pair into a broken rune.
		assert.True(t, utf8.ValidString(opt))
	})

	t.Run("truncated option still matches its book in winnersFromPoll", func(t *testing.T) {
		long := &models.Book{
			Title:  strings.Repeat("A very long title ", 10),
			Author: strings.Repeat("An author ", 10),
		}
		short := &models.Book{Title: "Dune", Author: "Herbert"}
		session := sessionWith(
			&models.Participant{SubscriberID: 1, Step: models.StepDone, Book: long},
			&models.Participant{SubscriberID: 2, Step: models.StepDone, Book: short},
		)

		// Telegram trims trailing whitespace from option texts it echoes back.
		poll := &tgbotapi.Poll{Options: []tgbotapi.PollOption{
			{Text: strings.TrimSpace(b.pollOptionFor(long)), VoterCount: 3},
			{Text: strings.TrimSpace(b.pollOptionFor(short)), VoterCount: 1},
		}}
		for _, o := range poll.Options {
			assert.LessOrEqual(t, utf16Len(o.Text), pollOptionMaxLen)
		}

		winners := b.winnersFromPoll(session, poll)
		assert.Equal(t, []models.Winner{{SubscriberID: 1, Title: long.Title, Author: long.Author}}, winners)
	})
}

func TestWinnersFromPollWithIdenticalOptionText(t *testing.T) {
	b := testBot()
	// Two volumes of the same work: the titles diverge only past the poll option
	// cap, so both books render to byte-identical option text.
	common := strings.Repeat("а", pollOptionMaxLen)
	volOne := &models.Book{Title: common + " том первый", Author: "Толстой"}
	volTwo := &models.Book{Title: common + " том второй", Author: "Толстой"}

	session := votingOn(sessionWith(
		&models.Participant{SubscriberID: 1, Step: models.StepDone, Book: volOne},
		&models.Participant{SubscriberID: 2, Step: models.StepDone, Book: volTwo},
	), 1, 2)

	// Guard the premise: without it this test would prove nothing.
	assert.Equal(t, b.pollOptionFor(volOne), b.pollOptionFor(volTwo), "test setup: options must collide")

	t.Run("the second option wins", func(t *testing.T) {
		poll := &tgbotapi.Poll{Options: []tgbotapi.PollOption{
			{Text: b.pollOptionFor(volOne), VoterCount: 1},
			{Text: b.pollOptionFor(volTwo), VoterCount: 4},
		}}
		winners := b.winnersFromPoll(session, poll)
		assert.Equal(t, []models.Winner{{SubscriberID: 2, Title: volTwo.Title, Author: "Толстой"}}, winners)
	})

	t.Run("the first option wins", func(t *testing.T) {
		poll := &tgbotapi.Poll{Options: []tgbotapi.PollOption{
			{Text: b.pollOptionFor(volOne), VoterCount: 4},
			{Text: b.pollOptionFor(volTwo), VoterCount: 1},
		}}
		winners := b.winnersFromPoll(session, poll)
		assert.Equal(t, []models.Winner{{SubscriberID: 1, Title: volOne.Title, Author: "Толстой"}}, winners)
	})
}

func TestWinnersFromPollFallsBackToTextMatching(t *testing.T) {
	b := testBot()
	dune := &models.Book{Title: "Dune", Author: "Herbert"}
	neuro := &models.Book{Title: "Neuromancer", Author: "Gibson"}

	// A poll opened before OptionOwners existed: the round was already in flight
	// when this version was deployed, so the owner list is missing.
	session := sessionWith(
		&models.Participant{SubscriberID: 1, Step: models.StepDone, Book: dune},
		&models.Participant{SubscriberID: 2, Step: models.StepDone, Book: neuro},
	)
	poll := &tgbotapi.Poll{Options: []tgbotapi.PollOption{
		{Text: strings.TrimSpace(b.pollOptionFor(dune)), VoterCount: 3},
		{Text: strings.TrimSpace(b.pollOptionFor(neuro)), VoterCount: 1},
	}}

	winners := b.winnersFromPoll(session, poll)
	assert.Equal(t, []models.Winner{{SubscriberID: 1, Title: "Dune", Author: "Herbert"}}, winners)
}

func TestWinnerAnnouncement(t *testing.T) {
	b := testBot()
	b.messages.WeHaveAWinner = "We have a winner"
	b.messages.NoClearWinnerManualVoting = "No clear winner"
	b.messages.ErrorDeterminingWinner = "Something went wrong"

	longTitle := strings.Repeat("A very long title ", 10)

	t.Run("a long winner is announced in full, not truncated", func(t *testing.T) {
		session := sessionWith(&models.Participant{SubscriberID: 1, Step: models.StepDone,
			Book: &models.Book{Title: longTitle, Author: "Herbert"}})
		txt := b.winnerAnnouncement(session, votedPoll(1), []models.Winner{{SubscriberID: 1, Title: longTitle, Author: "Herbert"}})

		assert.Contains(t, txt, longTitle)
		assert.Contains(t, txt, "Author: Herbert")
		assert.NotContains(t, txt, "…")
	})

	t.Run("a tie lists every winner", func(t *testing.T) {
		session := sessionWith()
		txt := b.winnerAnnouncement(session, votedPoll(2), []models.Winner{
			{SubscriberID: 1, Title: "Dune", Author: "Herbert"},
			{SubscriberID: 2, Title: "Neuromancer", Author: "Gibson"},
		})

		assert.Contains(t, txt, "No clear winner")
		assert.Contains(t, txt, "Book: Dune. Author: Herbert")
		assert.Contains(t, txt, "Book: Neuromancer. Author: Gibson")
	})

	t.Run("nobody voted offers every gathered book", func(t *testing.T) {
		session := sessionWith(
			&models.Participant{SubscriberID: 1, Step: models.StepDone, Book: &models.Book{Title: "Dune", Author: "Herbert"}},
			&models.Participant{SubscriberID: 2, Step: models.StepDone, Book: &models.Book{Title: "Neuromancer", Author: "Gibson"}},
			&models.Participant{SubscriberID: 3, Step: models.StepSkipped},
		)
		txt := b.winnerAnnouncement(session, emptyPoll(2), nil)

		assert.Contains(t, txt, "No clear winner")
		assert.Contains(t, txt, "Book: Dune. Author: Herbert")
		assert.Contains(t, txt, "Book: Neuromancer. Author: Gibson")
	})

	t.Run("no winner and nothing gathered reports an error", func(t *testing.T) {
		txt := b.winnerAnnouncement(sessionWith(), emptyPoll(0), nil)
		assert.Equal(t, "Something went wrong", txt)
	})
}

func TestWinnerAnnouncementStaysSendable(t *testing.T) {
	b := testBot()
	b.messages.NoClearWinnerManualVoting = "No clear winner"

	// Titles are unbounded member text, and a tie can list several of them.
	huge := strings.Repeat("я", 900)
	winners := make([]models.Winner, 0, 6)
	for i := 0; i < 6; i++ {
		winners = append(winners, models.Winner{SubscriberID: int64(i), Title: huge, Author: "Автор"})
	}

	txt := b.winnerAnnouncement(sessionWith(), votedPoll(6), winners)
	assert.LessOrEqual(t, utf16Len(txt), telegramMessageMaxLen)
}

func TestWinnerAnnouncementNoVotesOffersTheBallot(t *testing.T) {
	b := testBot()
	b.messages.NoClearWinnerManualVoting = "No clear winner"

	// Three finished submissions, but only two of them made it onto the ballot.
	session := votingOn(sessionWith(
		&models.Participant{SubscriberID: 1, Step: models.StepDone, Book: &models.Book{Title: "Dune", Author: "Herbert"}},
		&models.Participant{SubscriberID: 2, Step: models.StepDone, Book: &models.Book{Title: "Neuromancer", Author: "Gibson"}},
		&models.Participant{SubscriberID: 3, Step: models.StepDone, Book: &models.Book{Title: "Solaris", Author: "Lem"}},
	), 1, 2)

	txt := b.winnerAnnouncement(session, emptyPoll(2), nil)

	assert.Contains(t, txt, "Dune")
	assert.Contains(t, txt, "Neuromancer")
	assert.NotContains(t, txt, "Solaris", "offered a book that was never on the ballot")
}

func TestWinnerAnnouncementVotesButNoMatch(t *testing.T) {
	b := testBot()
	b.messages.ErrorDeterminingWinner = "Something went wrong"
	b.messages.NoClearWinnerManualVoting = "No clear winner"

	// The poll drew votes, but nothing resolved back to a book — a real failure,
	// which must not be reported as "nobody voted, pick one yourselves".
	session := votingOn(sessionWith(
		&models.Participant{SubscriberID: 1, Step: models.StepDone, Book: &models.Book{Title: "Dune", Author: "Herbert"}},
	), 1)

	txt := b.winnerAnnouncement(session, votedPoll(1), nil)

	assert.Equal(t, "Something went wrong", txt)
	assert.NotContains(t, txt, "No clear winner")
}
