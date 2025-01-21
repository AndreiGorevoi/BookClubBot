package bot

import (
	"BookClubBot/config"
	"BookClubBot/message"
	"BookClubBot/repository"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	cfg           *config.AppConfig
	tgBot         *tgbotapi.BotAPI
	bookGathering *bookGathering
	telegramPoll  *telegramPoll
	messages      *message.LocalizedMessages
	subRepository *repository.SubscriberRepository
}

func NewBot(cfg *config.AppConfig, messages *message.LocalizedMessages, subRepository *repository.SubscriberRepository) *Bot {
	return &Bot{
		cfg:           cfg,
		bookGathering: &bookGathering{},
		telegramPoll: &telegramPoll{
			voted: make(map[int64]struct{}),
		},
		messages:      messages,
		subRepository: subRepository,
	}
}

// Run stars telegram bot on using provided API key from a config
func (b *Bot) Run() {
	var err error
	b.tgBot, err = tgbotapi.NewBotAPI(b.cfg.TKey)

	if err != nil {
		log.Fatal(err)
	}

	b.tgBot.Debug = b.cfg.DebugMode

	u := tgbotapi.NewUpdate(0)
	u.Timeout = b.cfg.LongPollingTimeout

	updates := b.tgBot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			if update.Message.Chat.ID == b.cfg.GroupId {
				continue // ignore all messages from group
			}

			// handle msgs from users
			switch update.Message.Text {
			case "/subscribe":
				b.handleSubscription(&update)
			case "/start_vote":
				b.handleStartVote(&update)
			case "/skip":
				b.handleSkip(&update)
			default:
				b.handleUserMsg(&update)
			}
			continue
		}

		if update.PollAnswer != nil {
			if !b.telegramPoll.isActive {
				continue // do nothing as there is no active poll
			}

			// count all unique votes
			b.telegramPoll.voted[update.PollAnswer.User.ID] = struct{}{}
			if len(b.telegramPoll.voted) >= b.telegramPoll.participants {
				b.closeTelegramPoll()
			}
		}
	}
}

// handleSubscription handles /subscribe command from a user that adds them to subs
// if they are not subscribed yet
func (b *Bot) handleSubscription(update *tgbotapi.Update) {
	newSub := repository.Subscriber{
		Id:        update.Message.From.ID,
		Nick:      update.Message.From.UserName,
		FirstName: update.Message.From.FirstName,
		LastName:  update.Message.From.LastName,
	}
	err := b.subRepository.AddSubscriber(newSub)
	if errors.Is(err, repository.ErrUserAlreadySubscribed) {
		msg := tgbotapi.NewMessage(update.Message.From.ID, b.messages.AlreadySubscribedWaitForVoting)
		b.tgBot.Send(msg)
		return
	} else if err != nil {
		log.Println(err)
		return
	}
	msg := tgbotapi.NewMessage(update.Message.From.ID, b.messages.WelcomeBookClubNextVoting)
	b.tgBot.Send(msg)
}

// handleStartVote handles a starting a book gathering from subcribers
func (b *Bot) handleStartVote(update *tgbotapi.Update) {
	if b.bookGathering.active {
		msg := tgbotapi.NewMessage(update.Message.From.ID, b.messages.VotingAlreadyStartedWaitForEnd)
		b.tgBot.Send(msg)
		return
	}

	b.bookGathering.active = true
	err := b.initParticipants()
	if err != nil {
		log.Println(err)
		return
	}
	b.deadlineNotificationBookGathering(time.Duration(b.cfg.TimeToGatherBooks-b.cfg.NotifyBeforeGathering) * time.Second)
	b.runTelegramPollFlowAfterDelay(time.Duration(b.cfg.TimeToGatherBooks) * time.Second)
}

// handleUserMsg handles any message from a user
func (b *Bot) handleUserMsg(update *tgbotapi.Update) {
	var msg tgbotapi.MessageConfig
	currentUserId := update.Message.From.ID
	if !b.bookGathering.active {
		msg = tgbotapi.NewMessage(currentUserId, b.messages.VotingNotStartedOrEnded)
		b.tgBot.Send(msg)
		return
	}

	var particiapant *participant
	// check whether the particiapant is a participant of a poll
	for _, p := range b.bookGathering.participants {
		if p.id == currentUserId {
			particiapant = p
			break
		}
	}
	if particiapant == nil {
		msg = tgbotapi.NewMessage(currentUserId, b.messages.NotParticipantCurrentVoting)
		b.tgBot.Send(msg)
		return
	}

	b.handleParticipantAnswer(particiapant, update)

	if b.areAllBooksChosen() && b.bookGathering.active {
		b.runTelegramPollFlow()
	}
}

// handleParticipantAnswer handles an answer from a particiapant during a book gathering
func (b *Bot) handleParticipantAnswer(p *participant, update *tgbotapi.Update) {
	switch p.status {
	case bookAsked:
		bookTitle := update.Message.Text
		p.book = &book{
			title: bookTitle,
		}
		msg := tgbotapi.NewMessage(update.Message.From.ID, b.messages.WhoIsAuthor)
		b.tgBot.Send(msg)
		p.status = authorAsked
	case authorAsked:
		author := update.Message.Text
		p.book.author = author
		msg := tgbotapi.NewMessage(update.Message.From.ID, b.messages.WriteBookDescription)
		b.tgBot.Send(msg)
		p.status = descriptionAsked
	case descriptionAsked:
		desc := update.Message.Text
		p.book.description = desc
		msg := tgbotapi.NewMessage(update.Message.From.ID, b.messages.AttachCoverPhoto)
		b.tgBot.Send(msg)
		p.status = imageAsked
	case imageAsked:
		if update.Message.Photo != nil {
			photo := (update.Message.Photo)[len(update.Message.Photo)-1]
			p.book.photoId = photo.FileID

			msg := tgbotapi.NewMessage(update.Message.From.ID, b.messages.BookAddedToNextVoting)
			b.tgBot.Send(msg)
		} else {
			// If no photo is provided, skip to the next step
			msg := tgbotapi.NewMessage(update.Message.From.ID, b.messages.ImageMissingBookAdded)
			b.tgBot.Send(msg)
		}
		p.status = finished
	case finished:
		msg := tgbotapi.NewMessage(update.Message.From.ID, b.messages.VotingAlreadyCompleted)
		b.tgBot.Send(msg)
	}
}

// handleSkip handles a '/skip' message from a user due remove them from an ongoing book gathering
func (b *Bot) handleSkip(update *tgbotapi.Update) {
	userId := update.Message.From.ID
	if !b.bookGathering.active {
		msg := tgbotapi.NewMessage(userId, b.messages.VotingNotStartedOrEnded)
		b.tgBot.Send(msg)
		return
	}
	if !b.bookGathering.isParticipant(userId) {
		msg := tgbotapi.NewMessage(userId, b.messages.AlreadyDeclinedSuggestion)
		b.tgBot.Send(msg)
		return
	}

	b.bookGathering.removeParticipant(userId)
	msg := tgbotapi.NewMessage(userId, b.messages.UnableToSuggestBook)
	b.tgBot.Send(msg)
}

// initParticipants fills participants to the bookGathering filds by
// converting subscribers and sends them a message with asking a name of a book
func (b *Bot) initParticipants() error {
	subs, err := b.subRepository.GetAll()
	if err != nil {
		return err
	}
	participants := make([]*participant, 0, len(subs))
	for _, sub := range subs {
		msg := tgbotapi.NewMessage(sub.Id, b.messages.PleaseSuggestBookTitle)
		b.tgBot.Send(msg)
		p := &participant{
			id:        sub.Id,
			firstName: sub.FirstName,
			lastName:  sub.LastName,
			nick:      sub.Nick,
			status:    bookAsked,
		}

		participants = append(participants, p)
	}
	b.bookGathering.participants = participants
	return nil
}

// announceWinner defines a winner and sends a message to the group
func (b *Bot) announceWinner(poll *tgbotapi.Poll) {
	winners := defineWinners(poll)
	var txt string
	switch len(winners) {
	case 0:
		txt = b.messages.ErrorDeterminingWinner
	case 1:
		txt = fmt.Sprintf("%s - '%s'\n", b.messages.WeHaveAWinner, winners[0])
	default:
		txt = fmt.Sprintf("%s: %s\n", b.messages.NoClearWinnerManualVoting, strings.Join(winners, ","))
	}

	msg := tgbotapi.NewMessage(b.cfg.GroupId, txt)
	b.tgBot.Send(msg)
}

func (b *Bot) areAllBooksChosen() bool {
	for _, p := range b.bookGathering.participants {
		if p.status != finished {
			return false
		}
	}
	return true
}

// msgAboutGatheringBooks sends a message about books are going to be in a poll to the group chat
func (b *Bot) msgAboutGatheringBooks() {
	// Ensure there are participants with books
	if b.bookGathering == nil || len(b.bookGathering.participants) == 0 {
		log.Println("No participants or books found.")
		return
	}

	var mediaGroup []interface{}

	for _, participant := range b.bookGathering.participants {
		// Check if the participant has suggested a book
		if participant.book == nil {
			continue
		}

		// Add an image for the book
		bookImage := participant.bookImage()
		bookImage.Caption = participant.bookCaption()
		bookImage.ParseMode = "Markdown"
		mediaGroup = append(mediaGroup, bookImage)
	}

	// Check if there are any media items to send
	if len(mediaGroup) == 0 {
		log.Println("No books to send in the media group.")
		return
	}

	// Create the media group message
	msg := tgbotapi.NewMediaGroup(b.cfg.GroupId, mediaGroup)

	// Send the media group
	_, err := b.tgBot.Send(msg)
	if err != nil {
		log.Printf("Failed to send media group: %v\n", err)
		return
	}
}

// runTelegramPollFlowAfterDelay runs a runTelegramPollFlow but with delay in a separate goroutine
func (b *Bot) runTelegramPollFlowAfterDelay(delay time.Duration) {
	go func() {
		time.Sleep(delay)
		if b.bookGathering.active {
			b.runTelegramPollFlow()
		}
	}()
}

// runTelegramPollFlow stops a book gathering and runs a telegram poll
func (b *Bot) runTelegramPollFlow() {
	defer b.stopBookGathering()
	err := b.runTelegramPoll()
	if err != nil {
		log.Printf("cannot run poll: %v\n", err)
		return
	}

	b.msgAboutGatheringBooks()
	b.deadlineNotificationTelegramPoll(time.Duration(b.cfg.TimeForTelegramPoll-b.cfg.NotifyBeforePoll) * time.Second)
	b.closeTelegramPollAfterDelay(time.Duration(b.cfg.TimeForTelegramPoll) * time.Second)
}

// runTelegramPoll creates and starts a poll for choosing a book in the group
func (b *Bot) runTelegramPoll() error {
	if b.telegramPoll.isActive {
		return errors.New("cannot run a poll as there is a still active poll")
	}
	books := b.extractBooks()
	if len(books) < 2 {
		return errors.New("cannot run a poll as there is less than 2 books")
	}
	votingEnds := fmt.Sprintf(b.messages.VotingEndsInHours, (time.Duration(b.cfg.TimeForTelegramPoll) * time.Second).Hours())
	txt := fmt.Sprintf("%s.%s", b.messages.ChooseUpToTwoBooks, votingEnds)
	poll := tgbotapi.NewPoll(b.cfg.GroupId, txt, books...)
	poll.IsAnonymous = false
	poll.AllowsMultipleAnswers = true
	msg, err := b.tgBot.Send(poll)
	if err != nil {
		return err
	}
	subs, err := b.subRepository.GetAll()
	if err != nil {
		return err
	}

	b.telegramPoll.isActive = true
	b.telegramPoll.pollId = msg.MessageID
	b.telegramPoll.participants = len(subs)
	return nil
}

// closePollAfterDelay runs a gourotine that stops a poll after a given delay
func (b *Bot) closeTelegramPollAfterDelay(delay time.Duration) {
	go func() {
		time.Sleep(delay)
		b.closeTelegramPoll()
	}()
}

// closeTelegramPoll stops a poll and anounce the winner
func (b *Bot) closeTelegramPoll() {
	if !b.telegramPoll.isActive {
		log.Print("there is not an active poll, cannot close it")
		return
	}
	defer b.clearPoll()
	finishPoll := tgbotapi.StopPollConfig{
		BaseEdit: tgbotapi.BaseEdit{
			ChatID:    b.cfg.GroupId,         // The chat ID where the poll was sent
			MessageID: b.telegramPoll.pollId, // The message ID of the poll
		},
	}
	res, err := b.tgBot.StopPoll(finishPoll)
	if err != nil {
		log.Print(err)
		return
	}
	b.announceWinner(&res)
}

// extractBooks gains a slice of string from a books that participants suggested
func (b *Bot) extractBooks() []string {
	books := make([]string, 0, len(b.bookGathering.participants))

	for _, p := range b.bookGathering.participants {
		if p.book == nil {
			continue
		}
		name := fmt.Sprintf("%s: %s. %s: %s\n", b.messages.BookLabel, p.book.title, b.messages.AuthorLabel, p.book.author)
		books = append(books, name)
	}
	return books
}

// getPhotoId returns a tgbotapi.FileID from a participant
func (b *Bot) getPhotoId(p *participant) tgbotapi.FileID {
	if p.book.photoId != "" {
		return tgbotapi.FileID(p.book.photoId)
	}
	return ""
}

// clearPoll refreshes the state of a telegram poll
func (b *Bot) clearPoll() {
	b.telegramPoll = &telegramPoll{
		voted: make(map[int64]struct{}),
	}
}

// stopBookGathering stops a book gathering and clreas a bookGathering state
func (b *Bot) stopBookGathering() {
	b.bookGathering.active = false
	b.bookGathering = &bookGathering{}
}

// deadlineNotificationBookGathering run a separate goroutine that notifies users about a deadline
// of a book gathering. Delay is taken from a config
func (b *Bot) deadlineNotificationBookGathering(delay time.Duration) {
	go func() {
		time.Sleep(delay)
		if b.bookGathering.active {
			// send a messega to all participants that have not chosen yet
			for _, p := range b.bookGathering.participants {
				if p.status == finished {
					continue
				}
				//TODO: think about format (handle days, hours, minutes etc. depends on how much time left)
				txt := fmt.Sprintf(b.messages.BookSubmissionDeadline, (time.Duration(b.cfg.NotifyBeforeGathering) * time.Second).Hours())
				msg := tgbotapi.NewMessage(p.id, txt)
				b.tgBot.Send(msg)
			}
		}
	}()
}

// deadlineNotificationTelegramPoll run a separate goroutine that notifies users about a deadline
// of a telegram poll. Delay is taken from a config
func (b *Bot) deadlineNotificationTelegramPoll(delay time.Duration) {
	go func() {
		time.Sleep(delay)
		if !b.telegramPoll.isActive {
			return
		}
		txt := fmt.Sprintf(b.messages.VotingEndsInHours, (time.Duration(b.cfg.NotifyBeforePoll) * time.Second).Hours())
		msg := tgbotapi.NewMessage(b.cfg.GroupId, txt)
		b.tgBot.Send(msg)
	}()
}
