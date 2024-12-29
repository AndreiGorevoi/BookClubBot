package bot

import (
	"BookClubBot/config"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var subs []Subscriber
var currectPoll *BookPoll = &BookPoll{}

type Bot struct {
	cfg   *config.AppConfig
	tgBot *tgbotapi.BotAPI
}

func NewBot(cfg *config.AppConfig) *Bot {
	return &Bot{
		cfg: cfg,
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

	subs = loadSubs()

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
					b.handleParticipantChoice(&update)
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
	subs = append(subs, newSub)
	persistSubs(subs)
}

func (b *Bot) handleStartVote(update *tgbotapi.Update) {
	if currectPoll.IsStarted {
		msg := tgbotapi.NewMessage(update.Message.From.ID, "Голосование уже запущено, дождитесь окончания голосования")
		b.tgBot.Send(msg)
		return
	} else {
		b.initParticipants()
		currectPoll.IsStarted = true
	}
}

func (b *Bot) handleParticipantChoice(update *tgbotapi.Update) {
	var msg tgbotapi.MessageConfig
	userId := update.Message.From.ID
	if !currectPoll.IsStarted {
		msg = tgbotapi.NewMessage(userId, "Голосование еще не началось или уже закончилось!")
		b.tgBot.Send(msg)
		return
	} else {
		var user *Participant
		for _, p := range currectPoll.Participants {
			if p.Id == userId {
				user = p
				break
			}
		}
		if user == nil {
			msg = tgbotapi.NewMessage(userId, "Похоже ты не участник текущего голосования")
			b.tgBot.Send(msg)
			return
		}

		switch user.Status {
		case BOOK_IS_ASKED:
			book := update.Message.Text
			user.Book = &Book{
				Title: book,
			}
			user.Status = FINISHED
		case FINISHED:
			msg = tgbotapi.NewMessage(userId, "Ты уже закончил голосование!")
			b.tgBot.Send(msg)
			return
		}

		allVoted := true
		for _, p := range currectPoll.Participants {
			if p.Status != FINISHED {
				allVoted = false
				break
			}
		}

		if allVoted {
			// close poll
			currectPoll.IsStarted = false
			books := make([]string, 0, len(currectPoll.Participants))
			books = append(books, "Mock book")

			for _, p := range currectPoll.Participants {
				books = append(books, p.Book.Title)
			}

			poll := tgbotapi.NewPoll(b.cfg.GroupId, "Выбираем книгу", books...)
			poll.IsAnonymous = true
			msgid, _ := b.tgBot.Send(poll)

			b.closePollAfterDelay(msgid.MessageID, 10*time.Second)

			currectPoll = &BookPoll{}
		}
	}

}

func (b *Bot) initParticipants() {
	participants := make([]*Participant, 0, len(subs))
	for _, sub := range subs {
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
	currectPoll.Participants = participants
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

		b.defineWinner(&res)
	}()
}

func (b *Bot) defineWinner(res *tgbotapi.Poll) {
	m := make(map[int][]string)
	max := -1

	for _, o := range res.Options {
		if o.VoterCount > max {
			max = o.VoterCount
		}
		m[o.VoterCount] = append(m[o.VoterCount], o.Text)
	}

	winners := m[max]

	if len(winners) > 1 {
		txt := fmt.Sprintf("К сожаление выявить одного победителя не удалось! Так как Андрей очень ленивый и слабый программист, он не смог написать для меня логику, чтобы запустить еще одно голосование... Вам придется самостоятеьно запустить голосование и выбрать победителя из этих книг: %s\n", strings.Join(winners, ","))
		msg := tgbotapi.NewMessage(b.cfg.GroupId, txt)
		b.tgBot.Send(msg)
		return
	}

	txt := fmt.Sprintf("И у нас есть побелитель! Книгу которую мы будем читать - '%s'", winners[0])
	msg := tgbotapi.NewMessage(b.cfg.GroupId, txt)
	b.tgBot.Send(msg)
}
