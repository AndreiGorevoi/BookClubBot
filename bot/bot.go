package bot

import (
	"BookClubBot/config"
	"BookClubBot/internal/models"
	"BookClubBot/internal/repository"
	"BookClubBot/message"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.mongodb.org/mongo-driver/mongo"
)

type Bot struct {
	mu                 sync.Mutex
	cfg                *config.AppConfig
	tgBot              *tgbotapi.BotAPI
	bookGathering      *bookGathering
	telegramPoll       *telegramPoll
	messages           *message.LocalizedMessages
	subRepository      *repository.SubscriberRepository
	settingsRepository *repository.SettingsRepository
}

func NewBot(cfg *config.AppConfig, messages *message.LocalizedMessages, subRepository *repository.SubscriberRepository, settingsRepository *repository.SettingsRepository) *Bot {
	return &Bot{
		cfg:           cfg,
		bookGathering: &bookGathering{},
		telegramPoll: &telegramPoll{
			voted: make(map[int64]struct{}),
		},
		messages:           messages,
		subRepository:      subRepository,
		settingsRepository: settingsRepository,
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
	groupId, err := b.settingsRepository.GetGroupId(context.Background())
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			if err := b.settingsRepository.SaveGroupID(context.Background(), 0); err != nil {
				log.Fatalf("error during setting a group id: '%v'", err)
			}
		} else {
			log.Fatalf("unexpected error getting group id: '%v'", err)
		}
	}

	b.cfg.GroupId = int64(groupId)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = b.cfg.LongPollingTimeout

	updates := b.tgBot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			if update.Message.NewChatMembers != nil {
				b.handleBotAdded(update)
				continue
			}
			if update.Message.LeftChatMember != nil && update.Message.LeftChatMember.ID == b.tgBot.Self.ID {
				b.handleBotRemoved()
				continue
			}

			if update.Message.Chat.ID == b.cfg.GroupId {
				continue // ignore all messages from group
			}

			s, err := b.subRepository.GetSubscriberById(context.Background(), update.Message.Chat.ID)

			if err != nil {
				log.Printf("cannot execute 'FindById' from subRepository: %s", err)
				msg := tgbotapi.NewMessage(update.Message.From.ID, b.messages.SomethingWrong)
				b.tgBot.Send(msg)
				continue
			}

			// handle unsubscribed user's msg
			if update.Message.Text != "/subscribe" && (s == nil || s.Archived == true) {
				msg := tgbotapi.NewMessage(update.Message.From.ID, b.messages.NotSubscriber)
				b.tgBot.Send(msg)
				continue
			}

			// handle msgs from users
			switch update.Message.Text {
			case "/subscribe":
				b.processCommand(&update, b.handleSubsribe)
			case "/unsubscribe":
				b.processCommand(&update, b.handleUnsubscribe)
			case "/start_vote":
				b.processCommand(&update, b.handleStartVote)
			case "/skip":
				b.handleSkip(&update)
			case "/help":
				b.handleHelp(&update)
			default:
				b.handleUserMsg(&update)
			}
			continue
		}

		if update.PollAnswer != nil {
			b.mu.Lock()
			if !b.telegramPoll.isActive {
				b.mu.Unlock()
				continue
			}
			b.telegramPoll.voted[update.PollAnswer.User.ID] = struct{}{}
			shouldClose := len(b.telegramPoll.voted) >= b.telegramPoll.participants
			b.mu.Unlock()

			if shouldClose {
				b.closeTelegramPoll()
			}
		}
	}
}

func (b *Bot) handleBotRemoved() {
	err := b.settingsRepository.SaveGroupID(context.Background(), 0)
	if err != nil {
		log.Printf("cannot handle bot removing: %v", err)
		return
	}
	b.cfg.GroupId = 0
}

func (b *Bot) handleBotAdded(update tgbotapi.Update) {
	for _, member := range update.Message.NewChatMembers {
		if member.IsBot && member.ID == b.tgBot.Self.ID {
			groupId := update.Message.Chat.ID
			err := b.settingsRepository.SaveGroupID(context.Background(), groupId)
			if err != nil {
				log.Printf("cannot handle bot adding: %v", err)
				return
			}
			b.cfg.GroupId = groupId
			b.sendMessage(groupId, b.messages.GreetingMessage)
		}
	}
}

// handleSubsribe handles /subscribe command from a user that adds them to subs
// if they are not subscribed yet
func (b *Bot) handleSubsribe(update *tgbotapi.Update) error {
	uid := update.Message.From.ID
	s, err := b.subRepository.GetSubscriberById(context.Background(), uid)
	if err != nil {
		return fmt.Errorf("failed to find subscriber with id %d: %w", uid, err)
	}

	//case1: New subscriber (not found in DB)
	if s == nil {
		newSub := models.Subscriber{
			ID:        uid,
			Nick:      update.Message.From.UserName,
			FirstName: update.Message.From.FirstName,
			LastName:  update.Message.From.LastName,
			JoinedAt:  time.Now(),
		}
		err = b.subRepository.SaveSubscriber(context.Background(), &newSub)
		if err != nil {
			return fmt.Errorf("failed to add a new subscriber: %w", err)
		}
		b.sendMessage(uid, b.messages.WelcomeBookClubNextVoting)
		log.Printf("user %s %s subsribed\n", newSub.FirstName, newSub.LastName)
		return nil
	}

	//case2: Already subscribed and active
	if !s.Archived {
		msg := tgbotapi.NewMessage(update.Message.From.ID, b.messages.AlreadySubscribedWaitForVoting)
		b.tgBot.Send(msg)
		return nil
	}

	// case3: Reactivating archived subscriber
	if err := b.subRepository.SetArchiveSubscriber(context.Background(), uid, false); err != nil {
		return fmt.Errorf("failed to reactivate a subscriber with id %d : %w", uid, err)
	}
	b.sendMessage(uid, b.messages.WelcomeBack)
	log.Printf("user %s %s reactivated\n", s.FirstName, s.LastName)
	return nil
}

func (b *Bot) handleUnsubscribe(update *tgbotapi.Update) error {
	uid := update.Message.From.ID
	if err := b.subRepository.SetArchiveSubscriber(context.Background(), uid, true); err != nil {
		return fmt.Errorf("failed to unsubsride a user with id %d : %w", uid, err)
	}
	log.Printf("user with user id: %d unsubsribed", uid)
	b.sendMessage(uid, b.messages.Unsubsribed)
	return nil
}

// handleStartVote handles a starting a book gathering from subcribers
func (b *Bot) handleStartVote(update *tgbotapi.Update) error {
	if b.cfg.GroupId == 0 {
		b.sendMessage(update.Message.From.ID, b.messages.CannotStartGatheringGroupIdMissing)
		return nil
	}
	if b.bookGathering.active {
		b.sendMessage(update.Message.From.ID, b.messages.VotingAlreadyStartedWaitForEnd)
		return nil
	}

	b.bookGathering.active = true
	err := b.initParticipants()
	if err != nil {
		return fmt.Errorf("failed to init participants: %w", err)
	}
	b.deadlineNotificationBookGathering(time.Duration(b.cfg.TimeToGatherBooks-b.cfg.NotifyBeforeGathering) * time.Second)
	b.runTelegramPollFlowAfterDelay(time.Duration(b.cfg.TimeToGatherBooks) * time.Second)
	return nil
}

// handleUserMsg handles any message from a user
func (b *Bot) handleUserMsg(update *tgbotapi.Update) {
	currentUserId := update.Message.From.ID

	b.mu.Lock()
	active := b.bookGathering.active
	var participants []*participant
	if active {
		participants = make([]*participant, len(b.bookGathering.participants))
		copy(participants, b.bookGathering.participants)
	}
	b.mu.Unlock()

	if !active {
		b.sendMessage(currentUserId, b.messages.VotingNotStartedOrEnded)
		return
	}

	var particiapant *participant
	for _, p := range participants {
		if p.id == currentUserId {
			particiapant = p
			break
		}
	}
	if particiapant == nil {
		b.sendMessage(currentUserId, b.messages.NotParticipantCurrentVoting)
		return
	}

	b.handleParticipantAnswer(particiapant, update)

	b.mu.Lock()
	allDone := b.areAllBooksChosen()
	stillActive := b.bookGathering.active
	b.mu.Unlock()

	if allDone && stillActive {
		b.runTelegramPollFlow()
	}
}

// handleParticipantAnswer handles an answer from a particiapant during a book gathering
func (b *Bot) handleParticipantAnswer(p *participant, update *tgbotapi.Update) {
	b.mu.Lock()
	status := p.status
	b.mu.Unlock()

	switch status {
	case bookAsked:
		bookTitle := strings.TrimSpace(update.Message.Text)
		b.mu.Lock()
		alreadyProposed := b.isBookAlreadyProposed(bookTitle)
		if !alreadyProposed {
			p.book = &book{title: bookTitle}
			p.status = authorAsked
		}
		b.mu.Unlock()
		if alreadyProposed {
			b.sendMessage(update.Message.From.ID, b.messages.BookAlreadyProposed)
			return
		}
		b.sendMessage(update.Message.From.ID, b.messages.WhoIsAuthor)

	case authorAsked:
		b.mu.Lock()
		p.book.author = update.Message.Text
		p.status = descriptionAsked
		b.mu.Unlock()
		b.sendMessage(update.Message.From.ID, b.messages.WriteBookDescription)

	case descriptionAsked:
		b.mu.Lock()
		p.book.description = update.Message.Text
		p.status = imageAsked
		b.mu.Unlock()
		b.sendMessage(update.Message.From.ID, b.messages.AttachCoverPhoto)

	case imageAsked:
		b.mu.Lock()
		if update.Message.Photo != nil {
			photo := (update.Message.Photo)[len(update.Message.Photo)-1]
			p.book.photoId = photo.FileID
		}
		p.status = finished
		b.mu.Unlock()

		if update.Message.Photo != nil {
			b.sendMessage(update.Message.From.ID, b.messages.BookAddedToNextVoting)
		} else {
			b.sendMessage(update.Message.From.ID, b.messages.ImageMissingBookAdded)
		}
		log.Printf("user: %s %s suggested a book.\n", p.firstName, p.lastName)

	case finished:
		b.sendMessage(update.Message.From.ID, b.messages.VotingAlreadyCompleted)
	}
}

// handleSkip handles a '/skip' message from a user due remove them from an ongoing book gathering
func (b *Bot) handleSkip(update *tgbotapi.Update) {
	userId := update.Message.From.ID

	b.mu.Lock()
	active := b.bookGathering.active
	isParticipant := b.bookGathering.isParticipant(userId)
	b.mu.Unlock()

	if !active {
		b.sendMessage(userId, b.messages.VotingNotStartedOrEnded)
		return
	}
	if !isParticipant {
		b.sendMessage(userId, b.messages.AlreadyDeclinedSuggestion)
		return
	}

	b.mu.Lock()
	b.bookGathering.removeParticipant(userId)
	b.mu.Unlock()

	b.sendMessage(userId, b.messages.UnableToSuggestBook)
	log.Printf("user: %d skiped a book gathering.\n", userId)
}

// handleHelp handles a '/help' message from a user to give hime a help message about bot's functionality
func (b *Bot) handleHelp(update *tgbotapi.Update) {
	b.sendMessage(update.Message.From.ID, b.messages.HelpInfo)
}

// initParticipants fills participants to the bookGathering filds by
// converting subscribers and sends them a message with asking a name of a book
func (b *Bot) initParticipants() error {
	subs, err := b.subRepository.GetAllSubscribers(context.Background())
	if err != nil {
		return err
	}
	participants := make([]*participant, 0, len(subs))
	for _, sub := range subs {
		msg := tgbotapi.NewMessage(sub.ID, b.messages.PleaseSuggestBookTitle)
		b.tgBot.Send(msg)
		p := &participant{
			id:        sub.ID,
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
	if b.cfg.GroupId == 0 {
		log.Println("cannot announce winner as GroupId is not innit")
		return
	}
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
	if b.cfg.GroupId == 0 {
		log.Println("cannot send a msg about gathering books as GroupId is not innit")
		return
	}

	b.mu.Lock()
	groupId := b.cfg.GroupId
	var mediaItems []interface{}
	for _, p := range b.bookGathering.participants {
		if p.book == nil {
			continue
		}
		img := p.bookImage()
		img.Caption = truncateString(p.bookCaption(), 1024)
		img.ParseMode = "Markdown"
		mediaItems = append(mediaItems, img)
	}
	b.mu.Unlock()

	if len(mediaItems) == 0 {
		log.Println("No participants or books found.")
		return
	}

	for i := 0; i < len(mediaItems); i += 10 {
		end := i + 10
		if end > len(mediaItems) {
			end = len(mediaItems)
		}
		batch := mediaItems[i:end]
		if i == 0 && len(batch) < 2 {
			log.Println("cannot send message about gathered book as it less than 2")
			return
		}
		msg := tgbotapi.NewMediaGroup(groupId, batch)
		if _, err := b.tgBot.Send(msg); err != nil {
			log.Printf("ERROR: %s\n", err)
		}
	}
}

// runTelegramPollFlowAfterDelay runs a runTelegramPollFlow but with delay in a separate goroutine
func (b *Bot) runTelegramPollFlowAfterDelay(delay time.Duration) {
	go func() {
		time.Sleep(delay)
		b.mu.Lock()
		active := b.bookGathering.active
		b.mu.Unlock()
		if active {
			b.runTelegramPollFlow()
		}
	}()
}

// runTelegramPollFlow stops a book gathering and runs a telegram poll.
// It is safe to call concurrently: only the first call proceeds, subsequent
// calls that race with it see active==false and return immediately.
func (b *Bot) runTelegramPollFlow() {
	b.mu.Lock()
	if !b.bookGathering.active {
		b.mu.Unlock()
		return
	}
	b.bookGathering.active = false
	b.mu.Unlock()

	defer b.stopBookGathering()
	b.msgAboutGatheringBooks()

	err := b.runTelegramPoll()
	if err != nil {
		log.Printf("ERROR: cannot run poll: %v\n", err)
		return
	}

	b.deadlineNotificationTelegramPoll(time.Duration(b.cfg.TimeForTelegramPoll-b.cfg.NotifyBeforePoll) * time.Second)
	b.closeTelegramPollAfterDelay(time.Duration(b.cfg.TimeForTelegramPoll) * time.Second)
}

// runTelegramPoll creates and starts a poll for choosing a book in the group
func (b *Bot) runTelegramPoll() error {
	b.mu.Lock()
	if b.cfg.GroupId == 0 {
		b.mu.Unlock()
		return fmt.Errorf("cannot run telegram poll as groupId is not innit")
	}
	if b.telegramPoll.isActive {
		b.mu.Unlock()
		return errors.New("cannot run a poll as there is a still active poll")
	}
	b.mu.Unlock()

	books := b.extractBooks()
	if len(books) < 2 {
		return errors.New("cannot run a poll as there is less than 2 books")
	}

	if len(books) > 10 {
		log.Println("cannot use more than ten books in the poll... keeping the first ten")
		books = books[0:10]
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
	subs, err := b.subRepository.GetAllSubscribers(context.Background())
	if err != nil {
		return err
	}

	b.mu.Lock()
	b.telegramPoll.isActive = true
	b.telegramPoll.pollId = msg.MessageID
	b.telegramPoll.participants = len(subs)
	b.mu.Unlock()
	return nil
}

// closePollAfterDelay runs a gourotine that stops a poll after a given delay
func (b *Bot) closeTelegramPollAfterDelay(delay time.Duration) {
	go func() {
		time.Sleep(delay)
		b.closeTelegramPoll()
	}()
}

// closeTelegramPoll stops a poll and announces the winner.
// It is safe to call concurrently: only the first call proceeds, subsequent
// calls that race with it see isActive==false and return immediately.
func (b *Bot) closeTelegramPoll() {
	b.mu.Lock()
	if b.cfg.GroupId == 0 {
		b.mu.Unlock()
		log.Println("cannot close a telegram poll as GroupId is not innit")
		return
	}
	if !b.telegramPoll.isActive {
		b.mu.Unlock()
		log.Print("there is not an active poll, cannot close it")
		return
	}
	pollId := b.telegramPoll.pollId
	groupId := b.cfg.GroupId
	b.telegramPoll.isActive = false
	b.mu.Unlock()

	defer b.clearPoll()
	finishPoll := tgbotapi.StopPollConfig{
		BaseEdit: tgbotapi.BaseEdit{
			ChatID:    groupId,
			MessageID: pollId,
		},
	}
	res, err := b.tgBot.StopPoll(finishPoll)
	if err != nil {
		log.Printf("ERROR: %s", err)
		return
	}
	b.announceWinner(&res)
}

// extractBooks gains a slice of string from a books that participants suggested
func (b *Bot) extractBooks() []string {
	b.mu.Lock()
	books := make([]string, 0, len(b.bookGathering.participants))
	for _, p := range b.bookGathering.participants {
		if p.book != nil {
			name := fmt.Sprintf("%s: %s. %s: %s\n", b.messages.BookLabel, p.book.title, b.messages.AuthorLabel, p.book.author)
			books = append(books, name)
		}
	}
	b.mu.Unlock()
	return shuffleSlice(books)
}

// clearPoll refreshes the state of a telegram poll
func (b *Bot) clearPoll() {
	b.mu.Lock()
	b.telegramPoll = &telegramPoll{
		voted: make(map[int64]struct{}),
	}
	b.mu.Unlock()
}

// stopBookGathering stops a book gathering and clears a bookGathering state
func (b *Bot) stopBookGathering() {
	b.mu.Lock()
	b.bookGathering = &bookGathering{}
	b.mu.Unlock()
}

// deadlineNotificationBookGathering run a separate goroutine that notifies users about a deadline
// of a book gathering. Delay is taken from a config
func (b *Bot) deadlineNotificationBookGathering(delay time.Duration) {
	go func() {
		time.Sleep(delay)

		b.mu.Lock()
		if !b.bookGathering.active {
			b.mu.Unlock()
			return
		}
		toNotify := make([]int64, 0, len(b.bookGathering.participants))
		for _, p := range b.bookGathering.participants {
			if p.status != finished {
				toNotify = append(toNotify, p.id)
			}
		}
		b.mu.Unlock()

		txt := fmt.Sprintf(b.messages.BookSubmissionDeadline, (time.Duration(b.cfg.NotifyBeforeGathering) * time.Second).Hours())
		for _, id := range toNotify {
			b.sendMessage(id, txt)
		}
	}()
}

// deadlineNotificationTelegramPoll run a separate goroutine that notifies users about a deadline
// of a telegram poll. Delay is taken from a config
func (b *Bot) deadlineNotificationTelegramPoll(delay time.Duration) {
	go func() {
		time.Sleep(delay)

		b.mu.Lock()
		if b.cfg.GroupId == 0 || !b.telegramPoll.isActive {
			if b.cfg.GroupId == 0 {
				log.Println("cannot announce the deadline as GroupId is not innit")
			}
			b.mu.Unlock()
			return
		}
		groupId := b.cfg.GroupId
		b.mu.Unlock()

		txt := fmt.Sprintf(b.messages.VotingEndsInHours, (time.Duration(b.cfg.NotifyBeforePoll) * time.Second).Hours())
		b.sendMessage(groupId, txt)
	}()
}

// isBookAlreadyProposed checks weather a book with provided title is already proposed by another particiapnt
func (b *Bot) isBookAlreadyProposed(bookTitle string) bool {
	for _, p := range b.bookGathering.participants {
		if p.book != nil && p.book.title == bookTitle {
			return true
		}
	}
	return false
}

// sendMessage is a helper function that sends a message to a user
func (b *Bot) sendMessage(userID int64, text string) {
	msg := tgbotapi.NewMessage(userID, text)
	b.tgBot.Send(msg)
}

// processCommand is a wrapper function that helps to consolidate printing of Something Wrong messages
func (b *Bot) processCommand(update *tgbotapi.Update, handler func(*tgbotapi.Update) error) {
	if err := handler(update); err != nil {
		log.Printf("ERROR: %s", err)
		b.sendMessage(update.Message.From.ID, b.messages.SomethingWrong)
	}
}
