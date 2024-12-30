package bot

import (
	"BookClubBot/config"
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
	bookGathering *BookGathering
	subs          []Subscriber
}

func NewBot(cfg *config.AppConfig) *Bot {
	return &Bot{
		cfg:           cfg,
		bookGathering: &BookGathering{},
	}
}

func (b *Bot) Run() {
	var err error
	b.tgBot, err = tgbotapi.NewBotAPI(b.cfg.TKey)

	if err != nil {
		log.Fatal(err)
	}

	b.tgBot.Debug = true

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.tgBot.GetUpdatesChan(u)

	b.subs = loadSubs()

	for update := range updates {
		if update.Message != nil {
			if update.Message.Chat.ID == b.cfg.GroupId {
				if update.Poll == nil || update.PollAnswer == nil {
					continue // ignore all messages from group
				}

				// handle poll answer
			} else if update.Message != nil {
				switch update.Message.Text {
				case "/subscribe":
					b.handleSubscription(&update)
				case "/start_vote":
					b.handleStartVote(&update)
				default:
					b.handleUserMsg(&update)
				}
			}
		}

	}
}

func (b *Bot) handleSubscription(update *tgbotapi.Update) {
	newSub := Subscriber{
		Id:        update.Message.From.ID,
		Nick:      update.Message.From.UserName,
		FirstName: update.Message.From.FirstName,
		LastName:  update.Message.From.LastName,
	}
	b.subs = append(b.subs, newSub)
	persistSubs(b.subs)
	msg := tgbotapi.NewMessage(update.Message.From.ID, "Добро пожаловать в наш книжный клуб! Когда начнется следующее голосование за книгу, я приду к тебе за твоим вариантом :)")
	b.tgBot.Send(msg)
}

func (b *Bot) handleStartVote(update *tgbotapi.Update) {
	if b.bookGathering.IsStarted {
		msg := tgbotapi.NewMessage(update.Message.From.ID, "Голосование уже запущено, дождитесь окончания голосования")
		b.tgBot.Send(msg)
		return
	} else {
		b.initParticipants()
		b.bookGathering.IsStarted = true
	}
}

func (b *Bot) handleUserMsg(update *tgbotapi.Update) {
	var msg tgbotapi.MessageConfig
	currentUserId := update.Message.From.ID
	if !b.bookGathering.IsStarted {
		msg = tgbotapi.NewMessage(currentUserId, "Голосование еще не началось или уже закончилось!")
		b.tgBot.Send(msg)
		return
	}

	var particiapant *Participant
	// check whether the particiapant is a participant of a poll
	for _, p := range b.bookGathering.Participants {
		if p.Id == currentUserId {
			particiapant = p
			break
		}
	}
	if particiapant == nil {
		msg = tgbotapi.NewMessage(currentUserId, "Похоже ты не участник текущего голосования")
		b.tgBot.Send(msg)
		return
	}

	b.handleParticipantAnswer(particiapant, update)

	if b.isAllVoted() {
		b.msgAboutGatheringBooks()
		msgid, err := b.runTelegramPoll()
		if err != nil {
			log.Printf("cannot run poll: %v\n", err)
			return
		}

		b.closePollAfterDelay(msgid, 10*time.Second)
		b.closeBookGathering()
	}

}

func (b *Bot) initParticipants() {
	participants := make([]*Participant, 0, len(b.subs))
	for _, sub := range b.subs {
		msg := tgbotapi.NewMessage(sub.Id, "Пожалуйста, предложи название книги:")
		b.tgBot.Send(msg)
		p := &Participant{
			Id:        sub.Id,
			FirstName: sub.FirstName,
			LastName:  sub.LastName,
			Nick:      sub.Nick,
			Status:    BOOK_IS_ASKED,
		}

		participants = append(participants, p)
	}
	b.bookGathering.Participants = participants
}

func (b *Bot) closePollAfterDelay(messageId int, delay time.Duration) {
	go func() {
		time.Sleep(delay)
		finishPoll := tgbotapi.StopPollConfig{
			BaseEdit: tgbotapi.BaseEdit{
				ChatID:    b.cfg.GroupId, // The chat ID where the poll was sent
				MessageID: messageId,     // The message ID of the poll
			},
		}
		res, err := b.tgBot.StopPoll(finishPoll)
		if err != nil {
			log.Print(err)
			return
		}

		b.announceWinner(&res)
	}()
}

func (b *Bot) announceWinner(poll *tgbotapi.Poll) {
	winners := defineWinners(poll)
	var txt string
	switch len(winners) {
	case 0:
		txt = fmt.Sprint("Что-то пошло не так, не удалось определить победителя :(")
	case 1:
		txt = fmt.Sprintf("И у нас есть побелитель! Книгу которую мы будем читать - '%s'", winners[0])
	default:
		txt = fmt.Sprintf("К сожаление выявить одного победителя не удалось! Так как Андрей очень ленивый и слабый программист, он не смог написать для меня логику, чтобы запустить еще одно голосование... Вам придется самостоятеьно запустить голосование и выбрать победителя из этих книг: %s\n", strings.Join(winners, ","))
	}

	msg := tgbotapi.NewMessage(b.cfg.GroupId, txt)
	b.tgBot.Send(msg)
}

func (b *Bot) handleParticipantAnswer(p *Participant, update *tgbotapi.Update) {
	switch p.Status {
	case BOOK_IS_ASKED:
		book := update.Message.Text
		p.Book = &Book{
			Title: book,
		}
		msg := tgbotapi.NewMessage(update.Message.From.ID, "Кто автор этой книги?")
		b.tgBot.Send(msg)
		p.Status = AUTHOR_IS_ASKED
	case AUTHOR_IS_ASKED:
		author := update.Message.Text
		p.Book.Author = author
		msg := tgbotapi.NewMessage(update.Message.From.ID, "Напиши краткое описание книги.")
		b.tgBot.Send(msg)
		p.Status = DESCRIPTION_IS_ASKED
	case DESCRIPTION_IS_ASKED:
		desc := update.Message.Text
		p.Book.Description = desc
		msg := tgbotapi.NewMessage(update.Message.From.ID, "Прикрепи фотографию обложки. Использована будет только одна картинка.")
		b.tgBot.Send(msg)
		p.Status = IMAGE_IS_ASKED
	case IMAGE_IS_ASKED:
		if update.Message.Photo != nil {
			photo := (update.Message.Photo)[len(update.Message.Photo)-1]
			p.Book.PhotoId = photo.FileID

			msg := tgbotapi.NewMessage(update.Message.From.ID, "Готово! Я добавил твое книгу в следующее голосование.")
			b.tgBot.Send(msg)
		} else {
			// If no photo is provided, skip to the next step
			msg := tgbotapi.NewMessage(update.Message.From.ID, "Ты пропустил изображение. Я добавил твою книгу в следующее голосование.")
			b.tgBot.Send(msg)
		}
		p.Status = FINISHED
	case FINISHED:
		msg := tgbotapi.NewMessage(update.Message.From.ID, "Ты уже закончил голосование!")
		b.tgBot.Send(msg)
	}
}

func (b *Bot) isAllVoted() bool {
	for _, p := range b.bookGathering.Participants {
		if p.Status != FINISHED {
			return false
		}
	}
	return true
}

func (b *Bot) closeBookGathering() {
	b.bookGathering.IsStarted = false
	b.bookGathering = &BookGathering{}
}

func (b *Bot) msgAboutGatheringBooks() {
	// Ensure there are participants with books
	if b.bookGathering == nil || len(b.bookGathering.Participants) == 0 {
		log.Println("No participants or books found.")
		return
	}

	var mediaGroup []interface{}

	for _, participant := range b.bookGathering.Participants {
		// Check if the participant has suggested a book
		if participant.Book == nil {
			continue
		}

		// Create caption for the book
		caption := fmt.Sprintf(
			"📚 *Название*: %s\n👤 *Автор*: %s\n📝 *Описание*: %s",
			participant.Book.Title,
			participant.Book.Author,
			participant.Book.Description,
		)

		// Add an image for the book
		bookImage := tgbotapi.NewInputMediaPhoto(tgbotapi.FileID(participant.Book.PhotoId))
		bookImage.Caption = caption
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

func (b *Bot) runTelegramPoll() (int, error) {
	books := b.extractBooks()
	if len(books) < 2 {
		return 0, errors.New("cannot run a poll as there is less than 2 books")
	}
	poll := tgbotapi.NewPoll(b.cfg.GroupId, "Выбираем книгу", books...)
	poll.IsAnonymous = false
	poll.AllowsMultipleAnswers = true
	msg, err := b.tgBot.Send(poll)
	if err != nil {
		return 0, err
	}
	return msg.MessageID, nil
}

func (b *Bot) extractBooks() []string {
	books := make([]string, 0, len(b.bookGathering.Participants))
	books = append(books, "Книга: Властелин Колец. Автор: Джон Роуэл Толкин")

	for _, p := range b.bookGathering.Participants {
		if p.Book == nil {
			continue
		}
		name := fmt.Sprintf("Книга: %s. Автор: %s", p.Book.Title, p.Book.Author)
		books = append(books, name)
	}
	return books
}

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
