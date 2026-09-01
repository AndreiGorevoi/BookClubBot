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
//
// Reminders due during the configured quiet hours are held until the window
// ends rather than dropped, so a nightly point never silently swallows the last
// call.

// quietHours is a nightly window during which reminders are held rather than
// sent. A reminder that comes due inside it is not dropped: the counter is left
// untouched, so the next tick after the window ends still sees it as owed and
// sends it then. That matters most for the last call, which is the reminder that
// also DMs and the one a member can least afford to miss.
type quietHours struct {
	start, end int
	// loc is the timezone the window is expressed in. A nil loc disables quiet
	// hours entirely.
	loc *time.Location
}

// quietHoursFromConfig reads the window off the app config.
func (b *Bot) quietHoursFromConfig() quietHours {
	return quietHours{start: b.cfg.QuietHoursStart, end: b.cfg.QuietHoursEnd, loc: b.cfg.Location}
}

// covers reports whether t falls inside the window. The window wraps midnight
// when start > end (the usual 23:00-08:00 shape); equal bounds disable it.
func (q quietHours) covers(t time.Time) bool {
	if q.loc == nil || q.start == q.end {
		return false
	}
	h := t.In(q.loc).Hour()
	if q.start < q.end {
		return h >= q.start && h < q.end
	}
	return h >= q.start || h < q.end
}

// endsAfter returns the next moment the window closes at or after t. Only
// meaningful while covers(t) is true.
func (q quietHours) endsAfter(t time.Time) time.Time {
	local := t.In(q.loc)
	end := time.Date(local.Year(), local.Month(), local.Day(), q.end, 0, 0, 0, q.loc)
	if !end.After(local) {
		end = end.AddDate(0, 0, 1)
	}
	return end
}

// holds reports whether a reminder due at now should wait for the window to
// close. Holding is only safe while there is still a tick left afterwards to
// deliver it: once the window outlasts the deadline, waiting would drop the
// reminder rather than delay it — and the reminder at stake is usually the last
// call, the only one that also DMs. In that case we accept a message at an
// antisocial hour over no message at all.
func (q quietHours) holds(now, deadline time.Time) bool {
	return q.covers(now) && q.endsAfter(now).Before(deadline)
}

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
func planGatheringReminder(session *models.BookClubSession, interval time.Duration, quiet quietHours, now time.Time) gatheringReminderPlan {
	if interval <= 0 {
		return gatheringReminderPlan{}
	}

	// Hold anything due during quiet hours. The counter is deliberately left
	// alone so the reminder is merely delayed to the end of the window, not lost,
	// and a whole night's worth collapses into the single message the backlog
	// rule already produces. See quietHours.holds for why a window that outlasts
	// the deadline does not hold at all.
	if quiet.holds(now, session.Gathering.Deadline) {
		return gatheringReminderPlan{}
	}

	// Once the deadline has passed there is nothing left to hurry towards: this
	// same tick is about to end the gathering and start the poll, so a "last
	// call" here would be sent seconds before the round moves on. This is the
	// case after downtime spanning the whole gathering window.
	if !now.Before(session.Gathering.Deadline) {
		return gatheringReminderPlan{}
	}

	number := currentReminderNumber(session.CreatedAt, session.Gathering.Deadline, interval, now)
	if number <= session.Gathering.RemindersSent {
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
		dueCount:   number,
		lastCall:   remaining <= interval,
		recipients: recipients,
		remaining:  remaining,
	}
}

// currentReminderNumber returns which reminder is owed at now: 1 for the first
// scheduled reminder, 2 for the second, and so on, or 0 before any is due. It is
// compared against Gathering.RemindersSent to tell whether a further one has
// come due since the last one was sent.
//
// The schedule is walked backwards from the deadline. A point at or before the
// start of gathering is not scheduled at all — an interval that divides the
// window exactly would otherwise put a reminder at the very moment members were
// DMed the prompt.
func currentReminderNumber(start, deadline time.Time, interval time.Duration, now time.Time) int {
	if interval <= 0 {
		return 0
	}

	number := 0
	for point := deadline.Add(-interval); point.After(start); point = point.Add(-interval) {
		if point.After(now) {
			continue // not reached yet
		}
		number++
	}
	return number
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
	plan := planGatheringReminder(session, interval, b.quietHoursFromConfig(), now)
	if !plan.send {
		return
	}

	// The persisted counter is the source of truth, but a failed write would
	// otherwise let the next tick 15s later send the very same reminder again —
	// and on the last call that means DMing every pending member a second time.
	// This in-process marker closes that window; MongoDB still governs across
	// restarts. Only the recovery goroutine touches it.
	if b.lastReminder.sessionID == session.ID && b.lastReminder.number >= plan.dueCount {
		return
	}

	if err := b.sendGatheringReminderToGroup(plan); err != nil {
		// Leave the counter alone so the next tick retries. A point is worth
		// retrying because it stays due until the deadline.
		log.Printf("recovery: gathering reminder %d not sent, retrying next tick: %v", plan.dueCount, err)
		return
	}

	if plan.lastCall {
		// The last call also goes to DMs. Subscription is a private-chat
		// relationship — nothing checks that a subscriber is in the group — so a
		// mention alone is not guaranteed to reach them, and the DM lands in the
		// very thread where the submission flow lives. Best-effort per recipient:
		// one member who blocked the bot must not hold up the others or trigger a
		// resend to everyone.
		b.dmGatheringLastCall(plan)
	}

	b.lastReminder.sessionID, b.lastReminder.number = session.ID, plan.dueCount

	if err := b.sessionRepository.SetGatheringRemindersSent(context.Background(), session.ID, plan.dueCount); err != nil {
		log.Printf("recovery: cannot mark gathering reminder %d sent: %v", plan.dueCount, err)
	}
}

// sendGatheringReminderToGroup posts the reminder with mentions to the group. It
// returns an error only when the send is worth retrying; a missing GroupId is
// not, since it cannot resolve itself before the deadline.
func (b *Bot) sendGatheringReminderToGroup(plan gatheringReminderPlan) error {
	if b.cfg.GroupId == 0 {
		log.Println("cannot remind about book gathering as GroupId is not innit")
		return nil
	}

	txt := fmt.Sprintf(b.messages.GatheringReminder, b.formatRemaining(plan.remaining), mentionList(plan.recipients))
	msg := tgbotapi.NewMessage(b.cfg.GroupId, txt)
	// HTML rather than the Markdown used elsewhere in the bot: this message
	// embeds member-supplied names and nicknames, and HTML escaping is
	// unambiguous, whereas legacy Markdown has no dependable escape for a
	// nickname containing '_'.
	msg.ParseMode = tgbotapi.ModeHTML
	if _, err := b.tgBot.Send(msg); err != nil {
		return fmt.Errorf("cannot send gathering reminder to group: %w", err)
	}
	return nil
}

// dmGatheringLastCall DMs the members who have not submitted before the deadline.
func (b *Bot) dmGatheringLastCall(plan gatheringReminderPlan) {
	txt := fmt.Sprintf(b.messages.BookSubmissionDeadline, b.formatRemaining(plan.remaining))
	for _, p := range plan.recipients {
		b.sendMessage(p.SubscriberID, txt)
	}
}

// formatRemaining renders how long is left for the message copy. Below an hour
// it switches to minutes: a reminder held overnight or delivered after downtime
// can land minutes before the deadline, where whole hours would read "0".
func (b *Bot) formatRemaining(d time.Duration) string {
	if d < time.Hour {
		minutes := int(d / time.Minute)
		if minutes < 1 {
			minutes = 1
		}
		return fmt.Sprintf(b.messages.TimeLeftMinutes, minutes)
	}
	return fmt.Sprintf(b.messages.TimeLeftHours, int(d.Hours()))
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
