package bot

import (
	"BookClubBot/internal/models"
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Gathering reminders are anchored backwards from the gathering deadline: one is
// due at deadline-k*interval for every integer k >= 1 whose point still falls
// strictly inside the gathering window. Every reminder is a group message
// mentioning the members who have not submitted a book; the k == 1 reminder is
// the last call and additionally DMs them.
//
// Anchoring backwards rather than forwards from the start is what makes the
// *final* reminder land at a known distance from the deadline. Counting forwards
// would put it at an arbitrary offset depending on how the interval divides the
// window (a 24h window with a 7h interval would leave 3h; with a 5h interval,
// 4h), which is exactly where predictability matters most.

// gatheringReminderPlan is what one recovery tick should do about gathering
// reminders. Keeping the decision separate from the sending makes the schedule
// arithmetic testable without a Telegram client.
type gatheringReminderPlan struct {
	// send reports whether a reminder is due now. When false every other field
	// is meaningless.
	send bool
	// dueCount is the number of schedule points that have come due, to persist
	// as Gathering.RemindersSent. It can jump by more than one after downtime —
	// the backlog is deliberately collapsed into a single reminder.
	dueCount int
	// lastCall is true for the k == 1 reminder, the only one that also DMs.
	lastCall bool
	// recipients are the participants who have not submitted a book.
	recipients []*models.Participant
	// remaining is how long is left until the deadline, for the message copy.
	remaining time.Duration
}

// planGatheringReminder decides whether the tick at now owes a reminder for
// session. interval <= 0 disables gathering reminders entirely.
func planGatheringReminder(session *models.BookClubSession, interval time.Duration, now time.Time) gatheringReminderPlan {
	if interval <= 0 {
		return gatheringReminderPlan{}
	}

	// Once the deadline has passed there is nothing left to hurry towards: this
	// same tick is about to end the gathering and start the poll, so a "last
	// call" here would be sent seconds before the round moves on. This is the
	// case after downtime spanning the whole gathering window.
	if !now.Before(session.Gathering.Deadline) {
		return gatheringReminderPlan{}
	}

	due := dueReminderCount(session.CreatedAt, session.Gathering.Deadline, interval, now)
	if due <= session.Gathering.RemindersSent {
		return gatheringReminderPlan{}
	}

	recipients := pendingParticipants(session)
	if len(recipients) == 0 {
		// Nobody to nudge. Don't bump the counter either: this tick is about to
		// end the gathering anyway (allBooksChosen), so there is no later state to
		// protect from a double send.
		return gatheringReminderPlan{}
	}

	remaining := session.Gathering.Deadline.Sub(now) // > 0, guarded above

	return gatheringReminderPlan{
		send:       true,
		dueCount:   due,
		lastCall:   remaining <= interval,
		recipients: recipients,
		remaining:  remaining,
	}
}

// dueReminderCount counts the schedule points at deadline-k*interval (k >= 1)
// that fall strictly after start and are not in the future at now.
//
// The count is monotonic in now, which is what lets it double as the persisted
// "how many have been handled" marker.
func dueReminderCount(start, deadline time.Time, interval time.Duration, now time.Time) int {
	window := deadline.Sub(start)
	if interval <= 0 || window <= 0 {
		return 0
	}

	// A point must fall strictly after the start: k*interval < window. An interval
	// that divides the window exactly would otherwise schedule a reminder for the
	// very moment gathering began, when members have just been DMed the prompt.
	kMax := ceilDiv(window, interval) - 1
	if kMax < 1 {
		return 0
	}

	// A point is due once deadline-k*interval <= now, i.e. k >= remaining/interval.
	kMin := 1
	if remaining := deadline.Sub(now); remaining > 0 {
		if kMin = ceilDiv(remaining, interval); kMin < 1 {
			kMin = 1
		}
	}

	if kMin > kMax {
		return 0
	}
	return kMax - kMin + 1
}

// ceilDiv divides a by b, rounding up. Both are assumed positive.
func ceilDiv(a, b time.Duration) int {
	return int((a + b - 1) / b)
}

// pendingParticipants returns the participants who have neither submitted a book
// nor opted out. Members who skipped are never nudged.
func pendingParticipants(session *models.BookClubSession) []*models.Participant {
	var pending []*models.Participant
	for _, p := range session.Gathering.Participants {
		if p.Step != models.StepDone && p.Step != models.StepSkipped {
			pending = append(pending, p)
		}
	}
	return pending
}

// remindAboutGathering sends the reminder a tick is owed, if any, and records it.
func (b *Bot) remindAboutGathering(session *models.BookClubSession, now time.Time) {
	interval := time.Duration(b.cfg.GatheringReminderInterval) * time.Second
	plan := planGatheringReminder(session, interval, now)
	if !plan.send {
		return
	}

	b.sendGatheringReminderToGroup(plan)
	if plan.lastCall {
		// The last call also goes to DMs. Subscription is a private-chat
		// relationship — nothing checks that a subscriber is in the group — so a
		// mention alone is not guaranteed to reach them, and the DM lands in the
		// very thread where the submission flow lives.
		b.dmGatheringLastCall(plan)
	}

	if err := b.sessionRepository.SetGatheringRemindersSent(context.Background(), session.ID, plan.dueCount); err != nil {
		log.Printf("recovery: cannot mark gathering reminder %d sent: %v", plan.dueCount, err)
	}
}

// sendGatheringReminderToGroup posts the reminder with mentions to the group.
func (b *Bot) sendGatheringReminderToGroup(plan gatheringReminderPlan) {
	if b.cfg.GroupId == 0 {
		log.Println("cannot remind about book gathering as GroupId is not innit")
		return
	}

	txt := fmt.Sprintf(b.messages.GatheringReminder, plan.remaining.Hours(), mentionList(plan.recipients))
	msg := tgbotapi.NewMessage(b.cfg.GroupId, txt)
	// HTML rather than the Markdown used elsewhere in the bot: this message
	// embeds member-supplied names and nicknames, and HTML escaping is
	// unambiguous, whereas legacy Markdown has no dependable escape for a
	// nickname containing '_'.
	msg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.tgBot.Send(msg); err != nil {
		log.Printf("cannot send gathering reminder to group: %v", err)
	}
}

// dmGatheringLastCall DMs the members who have not submitted before the deadline.
func (b *Bot) dmGatheringLastCall(plan gatheringReminderPlan) {
	txt := fmt.Sprintf(b.messages.BookSubmissionDeadline, plan.remaining.Hours())
	for _, p := range plan.recipients {
		b.sendMessage(p.SubscriberID, txt)
	}
}

// mentionList renders participants as a comma-separated list of Telegram
// mentions, for a message sent with HTML parse mode.
func mentionList(participants []*models.Participant) string {
	mentions := make([]string, 0, len(participants))
	for _, p := range participants {
		mentions = append(mentions, mention(p))
	}
	return strings.Join(mentions, ", ")
}

// mention renders one participant as a Telegram mention. A public username
// mentions by name; everyone else gets an inline tg://user link, which notifies
// just the same and is the only option for a member without a username.
func mention(p *models.Participant) string {
	if p.Nick != "" {
		return "@" + escapeHTML(p.Nick)
	}
	return fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, p.SubscriberID, escapeHTML(displayName(p)))
}

// displayName is the participant's name as snapshotted at invite time, falling
// back to their id so a mention is never empty.
func displayName(p *models.Participant) string {
	name := strings.TrimSpace(p.FirstName + " " + p.LastName)
	if name == "" {
		return strconv.FormatInt(p.SubscriberID, 10)
	}
	return name
}

// escapeHTML escapes the three characters Telegram's HTML parse mode reserves.
func escapeHTML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
