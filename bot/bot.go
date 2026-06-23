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
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// errNotEnoughBooks signals that a poll cannot start because fewer than two
// books were gathered. runTelegramPollFlow uses it to notify the group instead
// of silently cancelling the round.
var errNotEnoughBooks = errors.New("cannot run a poll as there is less than 2 books")

type Bot struct {
	// mu serializes the phase transitions (gathering → voting → completed) so
	// that a deadline goroutine and the main update loop cannot both drive the
	// same transition. The session in MongoDB is the source of truth.
	mu                 sync.Mutex
	cfg                *config.AppConfig
	tgBot              *tgbotapi.BotAPI
	messages           *message.LocalizedMessages
	subRepository      subscriberRepo
	settingsRepository settingsRepo
	sessionRepository  sessionRepo
}

func NewBot(cfg *config.AppConfig, messages *message.LocalizedMessages, subRepository subscriberRepo, settingsRepository settingsRepo, sessionRepository sessionRepo) *Bot {
	return &Bot{
		cfg:                cfg,
		messages:           messages,
		subRepository:      subRepository,
		settingsRepository: settingsRepository,
		sessionRepository:  sessionRepository,
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
		if errors.Is(err, repository.ErrNotFound) {
			if err := b.settingsRepository.SaveGroupID(context.Background(), 0); err != nil {
				log.Fatalf("error during setting a group id: '%v'", err)
			}
		} else {
			log.Fatalf("unexpected error getting group id: '%v'", err)
		}
	}

	b.cfg.GroupId = int64(groupId)

	// Drive deadlines and resume any in-flight round from persisted state.
	b.startRecoveryLoop()

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
			b.handlePollAnswer(update.PollAnswer)
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

// handleStartVote opens a new book gathering session and DMs every active
// subscriber to suggest a book.
func (b *Bot) handleStartVote(update *tgbotapi.Update) error {
	if b.cfg.GroupId == 0 {
		b.sendMessage(update.Message.From.ID, b.messages.CannotStartGatheringGroupIdMissing)
		return nil
	}

	subs, err := b.subRepository.GetAllSubscribers(context.Background())
	if err != nil {
		return fmt.Errorf("failed to load subscribers: %w", err)
	}

	now := time.Now().UTC()
	participants := make([]*models.Participant, 0, len(subs))
	for _, sub := range subs {
		participants = append(participants, &models.Participant{
			SubscriberID: sub.ID,
			FirstName:    sub.FirstName,
			LastName:     sub.LastName,
			Nick:         sub.Nick,
			Step:         models.StepBook,
			InvitedAt:    now,
		})
	}

	session := &models.BookClubSession{
		Name:      now.Format("January 2006"),
		Status:    models.StatusGathering,
		CreatedBy: update.Message.From.ID,
		Gathering: models.Gathering{
			Deadline:     now.Add(time.Duration(b.cfg.TimeToGatherBooks) * time.Second),
			NotifyAt:     now.Add(time.Duration(b.cfg.TimeToGatherBooks-b.cfg.NotifyBeforeGathering) * time.Second),
			Participants: participants,
		},
	}

	if err := b.sessionRepository.CreateSession(context.Background(), session); err != nil {
		if errors.Is(err, repository.ErrActiveSessionExists) {
			b.sendMessage(update.Message.From.ID, b.messages.VotingAlreadyStartedWaitForEnd)
			return nil
		}
		return fmt.Errorf("failed to create session: %w", err)
	}

	for _, p := range participants {
		b.sendMessage(p.SubscriberID, b.messages.PleaseSuggestBookTitle)
	}

	// The recovery loop drives the gathering reminder and the move to voting
	// from the session's persisted deadlines.
	return nil
}

// handleUserMsg handles any free-text message from a user during book gathering.
func (b *Bot) handleUserMsg(update *tgbotapi.Update) {
	uid := update.Message.From.ID

	session, err := b.sessionRepository.GetActiveSession(context.Background())
	if err != nil {
		log.Printf("cannot get active session: %v", err)
		b.sendMessage(uid, b.messages.SomethingWrong)
		return
	}
	if session == nil || session.Status != models.StatusGathering {
		b.sendMessage(uid, b.messages.VotingNotStartedOrEnded)
		return
	}

	p := findParticipant(session, uid)
	if p == nil || p.Step == models.StepSkipped {
		b.sendMessage(uid, b.messages.NotParticipantCurrentVoting)
		return
	}

	b.handleParticipantAnswer(session, p, update)

	// p points into session.Gathering.Participants, so session already reflects
	// the step just applied — no need to reload. If everyone has finished or
	// skipped, move straight to the poll.
	if allBooksChosen(session) {
		b.runTelegramPollFlow()
	}
}

// handleParticipantAnswer advances one participant's submission flow and
// persists each step.
func (b *Bot) handleParticipantAnswer(session *models.BookClubSession, p *models.Participant, update *tgbotapi.Update) {
	uid := update.Message.From.ID

	switch p.Step {
	case models.StepBook:
		title := strings.TrimSpace(update.Message.Text)
		if isBookAlreadyProposed(session, title) {
			b.sendMessage(uid, b.messages.BookAlreadyProposed)
			return
		}
		p.Book = &models.Book{Title: title}
		p.Step = models.StepAuthor
		b.persistParticipant(session.ID, p)
		b.sendMessage(uid, b.messages.WhoIsAuthor)

	case models.StepAuthor:
		p.Book.Author = update.Message.Text
		p.Step = models.StepDescription
		b.persistParticipant(session.ID, p)
		b.sendMessage(uid, b.messages.WriteBookDescription)

	case models.StepDescription:
		p.Book.Description = update.Message.Text
		p.Step = models.StepImage
		b.persistParticipant(session.ID, p)
		b.sendMessage(uid, b.messages.AttachCoverPhoto)

	case models.StepImage:
		hasPhoto := update.Message.Photo != nil
		if hasPhoto {
			photo := (update.Message.Photo)[len(update.Message.Photo)-1]
			p.Book.PhotoID = photo.FileID
		}
		now := time.Now().UTC()
		p.Step = models.StepDone
		p.SubmittedAt = &now
		b.persistParticipant(session.ID, p)

		if hasPhoto {
			b.sendMessage(uid, b.messages.BookAddedToNextVoting)
		} else {
			b.sendMessage(uid, b.messages.ImageMissingBookAdded)
		}
		log.Printf("user: %s %s suggested a book.\n", p.FirstName, p.LastName)

	case models.StepDone:
		b.sendMessage(uid, b.messages.VotingAlreadyCompleted)
	}
}

// handleSkip removes a user from the ongoing book gathering.
func (b *Bot) handleSkip(update *tgbotapi.Update) {
	uid := update.Message.From.ID

	session, err := b.sessionRepository.GetActiveSession(context.Background())
	if err != nil {
		log.Printf("cannot get active session: %v", err)
		b.sendMessage(uid, b.messages.SomethingWrong)
		return
	}
	if session == nil || session.Status != models.StatusGathering {
		b.sendMessage(uid, b.messages.VotingNotStartedOrEnded)
		return
	}

	p := findParticipant(session, uid)
	if p == nil || p.Step == models.StepSkipped {
		b.sendMessage(uid, b.messages.AlreadyDeclinedSuggestion)
		return
	}

	p.Step = models.StepSkipped
	p.Book = nil
	b.persistParticipant(session.ID, p)
	b.sendMessage(uid, b.messages.UnableToSuggestBook)
	log.Printf("user: %d skiped a book gathering.\n", uid)

	// session already reflects the skip (p points into it). The last pending
	// user skipping should end the gathering too.
	if allBooksChosen(session) {
		b.runTelegramPollFlow()
	}
}

// handleHelp handles a '/help' message from a user to give hime a help message about bot's functionality
func (b *Bot) handleHelp(update *tgbotapi.Update) {
	b.sendMessage(update.Message.From.ID, b.messages.HelpInfo)
}

// handlePollAnswer records a vote and closes the poll once everyone has voted.
func (b *Bot) handlePollAnswer(answer *tgbotapi.PollAnswer) {
	session, err := b.sessionRepository.GetActiveSession(context.Background())
	if err != nil {
		log.Printf("cannot get active session for poll answer: %v", err)
		return
	}
	if session == nil || session.Status != models.StatusVoting || session.Voting == nil {
		return
	}

	if err := b.sessionRepository.AddVoter(context.Background(), session.ID, answer.User.ID); err != nil {
		log.Printf("cannot record voter: %v", err)
		return
	}

	updated, err := b.sessionRepository.GetActiveSession(context.Background())
	if err != nil || updated == nil || updated.Voting == nil {
		return
	}
	if updated.Voting.TotalParticipants > 0 && len(updated.Voting.VoterIDs) >= updated.Voting.TotalParticipants {
		b.closeTelegramPoll()
	}
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

// msgAboutGatheringBooks sends a media group of the gathered books to the group.
func (b *Bot) msgAboutGatheringBooks(session *models.BookClubSession) {
	if b.cfg.GroupId == 0 {
		log.Println("cannot send a msg about gathering books as GroupId is not innit")
		return
	}

	groupId := b.cfg.GroupId
	var mediaItems []interface{}
	for _, p := range session.Gathering.Participants {
		if p.Step != models.StepDone || p.Book == nil {
			continue
		}
		vp := viewParticipant(p)
		img := vp.bookImage()
		img.Caption = truncateString(vp.bookCaption(), 1024)
		img.ParseMode = "Markdown"
		mediaItems = append(mediaItems, img)
	}

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

// runTelegramPollFlow ends the active book gathering and starts a telegram poll.
// It claims the transition under b.mu by flipping the session to the voting
// status; only the first caller that sees a gathering session proceeds.
func (b *Bot) runTelegramPollFlow() {
	b.mu.Lock()
	session, err := b.sessionRepository.GetActiveSession(context.Background())
	if err != nil {
		b.mu.Unlock()
		log.Printf("cannot get active session: %v", err)
		return
	}
	if session == nil || session.Status != models.StatusGathering {
		b.mu.Unlock()
		return
	}
	if err := b.sessionRepository.SetStatus(context.Background(), session.ID, models.StatusVoting); err != nil {
		b.mu.Unlock()
		log.Printf("cannot transition session to voting: %v", err)
		return
	}
	b.mu.Unlock()

	b.msgAboutGatheringBooks(session)

	if err := b.runTelegramPoll(session); err != nil {
		log.Printf("ERROR: cannot run poll: %v\n", err)
		// Could not start a poll. End the round so a new one can be started, and
		// tell the group why when the cause is too few books (rather than
		// silently cancelling).
		if errors.Is(err, errNotEnoughBooks) && b.cfg.GroupId != 0 {
			b.sendMessage(b.cfg.GroupId, b.messages.NotEnoughBooksVotingCancelled)
		}
		if err := b.sessionRepository.SetStatus(context.Background(), session.ID, models.StatusCancelled); err != nil {
			log.Printf("cannot cancel session: %v", err)
		}
		return
	}

	// The recovery loop drives the poll reminder and the close from the voting
	// sub-document's persisted deadline.
}

// runTelegramPoll creates and starts a poll for choosing a book in the group,
// then persists the voting sub-document.
func (b *Bot) runTelegramPoll(session *models.BookClubSession) error {
	if b.cfg.GroupId == 0 {
		return fmt.Errorf("cannot run telegram poll as groupId is not innit")
	}

	books := b.extractBooks(session)
	if len(books) < 2 {
		return errNotEnoughBooks
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

	now := time.Now().UTC()
	voting := &models.Voting{
		TelegramPollID:    msg.MessageID,
		Deadline:          now.Add(time.Duration(b.cfg.TimeForTelegramPoll) * time.Second),
		NotifyAt:          now.Add(time.Duration(b.cfg.TimeForTelegramPoll-b.cfg.NotifyBeforePoll) * time.Second),
		TotalParticipants: len(subs),
		StartedAt:         now,
	}
	return b.sessionRepository.StartVoting(context.Background(), session.ID, voting)
}

// closeTelegramPoll stops the poll, records the winner(s) and completes the
// session. The state-changing section runs under b.mu so a deadline tick and an
// all-voted close cannot both drive it. The session is completed only AFTER
// StopPoll succeeds: a failed StopPoll leaves it in voting so the close can be
// retried (by a later vote, or the recovery loop) instead of stranding an open
// poll with no winner and a held active lock. announceWinner runs after the
// lock is released — it is a plain group message and need not hold the lock.
func (b *Bot) closeTelegramPoll() {
	b.mu.Lock()
	if b.cfg.GroupId == 0 {
		b.mu.Unlock()
		log.Println("cannot close a telegram poll as GroupId is not innit")
		return
	}
	session, err := b.sessionRepository.GetActiveSession(context.Background())
	if err != nil {
		b.mu.Unlock()
		log.Printf("cannot get active session: %v", err)
		return
	}
	if session == nil || session.Status != models.StatusVoting || session.Voting == nil {
		b.mu.Unlock()
		log.Print("there is not an active poll, cannot close it")
		return
	}

	finishPoll := tgbotapi.StopPollConfig{
		BaseEdit: tgbotapi.BaseEdit{
			ChatID:    b.cfg.GroupId,
			MessageID: session.Voting.TelegramPollID,
		},
	}
	res, err := b.tgBot.StopPoll(finishPoll)
	if err != nil {
		b.mu.Unlock()
		log.Printf("ERROR: %s", err)
		return
	}

	if winners := b.winnersFromPoll(session, &res); len(winners) > 0 {
		if err := b.sessionRepository.SetWinners(context.Background(), session.ID, winners); err != nil {
			log.Printf("cannot save winners: %v", err)
		}
	}
	if err := b.sessionRepository.SetVotingClosed(context.Background(), session.ID, time.Now().UTC()); err != nil {
		log.Printf("cannot stamp poll close time: %v", err)
	}
	if err := b.sessionRepository.SetStatus(context.Background(), session.ID, models.StatusCompleted); err != nil {
		b.mu.Unlock()
		log.Printf("cannot complete session: %v", err)
		return
	}
	b.mu.Unlock()

	b.announceWinner(&res)
}

// extractBooks builds the shuffled poll options from the finished submissions.
func (b *Bot) extractBooks(session *models.BookClubSession) []string {
	books := make([]string, 0, len(session.Gathering.Participants))
	for _, p := range session.Gathering.Participants {
		if p.Step == models.StepDone && p.Book != nil {
			books = append(books, b.pollOptionFor(p.Book))
		}
	}
	return shuffleSlice(books)
}

// winnersFromPoll maps the winning poll option texts back to the books that
// produced them.
func (b *Bot) winnersFromPoll(session *models.BookClubSession, poll *tgbotapi.Poll) []models.Winner {
	if poll == nil {
		return nil
	}
	// A poll that closed with zero votes has max voter count 0, which
	// defineWinners reports as "every option won". Treat no votes as no winner.
	hasVotes := false
	for _, o := range poll.Options {
		if o.VoterCount > 0 {
			hasVotes = true
			break
		}
	}
	if !hasVotes {
		return nil
	}

	texts := defineWinners(poll)
	if len(texts) == 0 {
		return nil
	}
	// Telegram may trim trailing whitespace/newlines from option texts, so match
	// on trimmed values rather than exact equality.
	wonText := make(map[string]struct{}, len(texts))
	for _, t := range texts {
		wonText[strings.TrimSpace(t)] = struct{}{}
	}
	winners := make([]models.Winner, 0, len(texts))
	for _, p := range session.Gathering.Participants {
		if p.Step != models.StepDone || p.Book == nil {
			continue
		}
		if _, ok := wonText[strings.TrimSpace(b.pollOptionFor(p.Book))]; ok {
			winners = append(winners, models.Winner{
				SubscriberID: p.SubscriberID,
				Title:        p.Book.Title,
				Author:       p.Book.Author,
			})
		}
	}
	return winners
}

// pollOptionFor renders the poll option text for a book. The same rendering is
// used to build the poll and to match winners back to books, so they must stay
// in sync.
func (b *Bot) pollOptionFor(bk *models.Book) string {
	return fmt.Sprintf("%s: %s. %s: %s\n", b.messages.BookLabel, bk.Title, b.messages.AuthorLabel, bk.Author)
}

// notifyGatheringDeadline messages participants who have not finished before
// the gathering deadline.
func (b *Bot) notifyGatheringDeadline(session *models.BookClubSession) {
	txt := fmt.Sprintf(b.messages.BookSubmissionDeadline, (time.Duration(b.cfg.NotifyBeforeGathering) * time.Second).Hours())
	for _, p := range session.Gathering.Participants {
		if p.Step != models.StepDone && p.Step != models.StepSkipped {
			b.sendMessage(p.SubscriberID, txt)
		}
	}
}

// notifyPollDeadline messages the group before the poll deadline.
func (b *Bot) notifyPollDeadline() {
	if b.cfg.GroupId == 0 {
		log.Println("cannot announce the deadline as GroupId is not innit")
		return
	}
	txt := fmt.Sprintf(b.messages.VotingEndsInHours, (time.Duration(b.cfg.NotifyBeforePoll) * time.Second).Hours())
	b.sendMessage(b.cfg.GroupId, txt)
}

// findParticipant returns the participant with the given id, or nil.
func findParticipant(session *models.BookClubSession, id int64) *models.Participant {
	for _, p := range session.Gathering.Participants {
		if p.SubscriberID == id {
			return p
		}
	}
	return nil
}

// isBookAlreadyProposed reports whether a book with the given title was already
// proposed by any participant.
func isBookAlreadyProposed(session *models.BookClubSession, title string) bool {
	for _, p := range session.Gathering.Participants {
		if p.Book != nil && p.Book.Title == title {
			return true
		}
	}
	return false
}

// allBooksChosen reports whether every participant has finished or skipped.
func allBooksChosen(session *models.BookClubSession) bool {
	for _, p := range session.Gathering.Participants {
		if p.Step != models.StepDone && p.Step != models.StepSkipped {
			return false
		}
	}
	return true
}

// persistParticipant writes a participant's updated state, logging on failure.
func (b *Bot) persistParticipant(id primitive.ObjectID, p *models.Participant) {
	if err := b.sessionRepository.UpdateParticipant(context.Background(), id, p); err != nil {
		log.Printf("cannot update participant %d: %v", p.SubscriberID, err)
	}
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
