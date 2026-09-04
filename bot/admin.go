package bot

import (
	"BookClubBot/internal/models"
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Admin console callback payloads. Every one carries the "a:" prefix so
// handleCallback can route them before it looks for an active session — the
// console works with no round in flight. The longest payload is a prefix plus a
// Telegram user id, well inside the 64-byte callback_data limit.
const (
	callbackAdminRoot = "a:root"

	callbackAdminSession = "a:session"

	callbackAdminEndAsk     = "a:end"
	callbackAdminEndConfirm = "a:endgo"

	// Prefixes completed with an argument: a page number for the lists, a
	// subscriber id for the unsubscribe steps.
	callbackAdminMembersPage  = "a:members:"
	callbackAdminUnsubPage    = "a:unsub:"
	callbackAdminUnsubAsk     = "a:unsubask:"
	callbackAdminUnsubConfirm = "a:unsubgo:"
)

const (
	// adminPageSize is how many members one page of the console lists. Kept
	// small because the unsubscribe view spends one button row per member.
	adminPageSize = 8

	// adminNameMaxLen and adminButtonNameMaxLen bound member-supplied names in
	// the panel body and on a button. The panel is plain text (no parse mode),
	// so names need no escaping — only bounding, so that one long name cannot
	// push the message past Telegram's limit.
	adminNameMaxLen       = 48
	adminButtonNameMaxLen = 32
)

// adminView is one screen of the console: the panel text and the keyboard that
// drives it. Rendering a view never mutates anything, so a stale button can
// always be re-rendered safely.
type adminView struct {
	text     string
	keyboard *tgbotapi.InlineKeyboardMarkup
}

// handleAdmin opens the admin console in the caller's DM. Only admins may run
// it; the same check is repeated on every button press, since the panel stays
// in chat history long after it was opened.
func (b *Bot) handleAdmin(update *tgbotapi.Update) error {
	uid := update.Message.From.ID
	if !b.isAdmin(uid) {
		log.Printf("/admin denied for non-admin user %d", uid)
		b.sendMessage(uid, b.messages.AdminOnly)
		return nil
	}

	view := b.adminRootView()
	b.sendWithKeyboard(uid, view.text, view.keyboard)
	return nil
}

// handleAdminCallback routes an "a:" button press. Authorization is re-checked
// here rather than trusted from panel-open time: the buttons persist in the
// chat, and an admin can be removed from the config while their old panel is
// still on screen.
func (b *Bot) handleAdminCallback(cq *tgbotapi.CallbackQuery) {
	if !b.isAdmin(cq.From.ID) {
		log.Printf("admin callback %q denied for non-admin user %d", cq.Data, cq.From.ID)
		b.answerCallback(cq.ID, b.messages.AdminOnly)
		return
	}

	view, toast, known := b.adminRoute(cq.Data)
	if !known {
		b.answerCallback(cq.ID, "") // payload from an older version; just stop the spinner
		return
	}

	b.editPanel(cq, view)
	b.answerCallback(cq.ID, toast)
}

// adminRoute maps a callback payload to the view to show and the toast to pop.
// Payloads that perform something (ending the round, unsubscribing a member) do
// it here and then return the view the admin lands on afterwards. A false
// third result means the payload is not one of ours.
func (b *Bot) adminRoute(data string) (adminView, string, bool) {
	switch {
	case data == callbackAdminRoot:
		return b.adminRootView(), "", true
	case strings.HasPrefix(data, callbackAdminMembersPage):
		return b.adminMembersView(pageArg(data, callbackAdminMembersPage)), "", true
	case data == callbackAdminSession:
		return b.adminSessionView(), "", true
	case data == callbackAdminEndAsk:
		return b.adminEndConfirmView(), "", true
	case data == callbackAdminEndConfirm:
		view, toast := b.endActiveSession()
		return view, toast, true
	case strings.HasPrefix(data, callbackAdminUnsubPage):
		return b.adminUnsubListView(pageArg(data, callbackAdminUnsubPage)), "", true
	case strings.HasPrefix(data, callbackAdminUnsubAsk):
		id, ok := idArg(data, callbackAdminUnsubAsk)
		if !ok {
			return b.adminUnsubListView(0), "", true
		}
		return b.adminUnsubConfirmView(id), "", true
	case strings.HasPrefix(data, callbackAdminUnsubConfirm):
		id, ok := idArg(data, callbackAdminUnsubConfirm)
		if !ok {
			return b.adminUnsubListView(0), "", true
		}
		view, toast := b.unsubscribeMember(id)
		return view, toast, true
	}
	return adminView{}, "", false
}

// adminRootView is the console's home screen: a one-line summary of the current
// round plus the four actions.
func (b *Bot) adminRootView() adminView {
	session, err := b.sessionRepository.GetActiveSession(context.Background())

	var status string
	switch {
	case err != nil:
		log.Printf("admin: cannot load active session: %v", err)
		status = b.messages.AdminStatusUnknown
	case session == nil:
		status = b.messages.AdminNoActiveSession
	default:
		status = fmt.Sprintf(b.messages.AdminStatusLine, session.Name, b.phaseLabel(session.Status))
	}

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.messages.AdminBtnMembers, callbackAdminMembersPage+"0"),
			tgbotapi.NewInlineKeyboardButtonData(b.messages.AdminBtnSession, callbackAdminSession),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.messages.AdminBtnUnsubscribe, callbackAdminUnsubPage+"0"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.messages.AdminBtnEndRound, callbackAdminEndAsk),
		),
	)
	return adminView{text: fmt.Sprintf(b.messages.AdminPanelTitle, status), keyboard: &kb}
}

// adminMembersView lists the active subscribers, one page at a time.
func (b *Bot) adminMembersView(page int) adminView {
	subs, err := b.activeSubscribers()
	if err != nil {
		return b.adminErrorView()
	}
	if len(subs) == 0 {
		return adminView{text: b.messages.AdminMembersEmpty, keyboard: backKeyboard(b.messages.AdminBtnBack)}
	}

	start, end, pages, page := paginate(len(subs), page, adminPageSize)

	lines := []string{fmt.Sprintf(b.messages.AdminMembersTitle, len(subs)), ""}
	for i, s := range subs[start:end] {
		lines = append(lines, fmt.Sprintf("%d. %s", start+i+1, memberLabel(s, adminNameMaxLen)))
	}
	if pages > 1 {
		lines = append(lines, "", fmt.Sprintf(b.messages.AdminMembersPage, page+1, pages))
	}

	rows := b.pagerRows(callbackAdminMembersPage, page, pages)
	rows = append(rows, backRow(b.messages.AdminBtnBack))
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return adminView{text: panelText(lines), keyboard: &kb}
}

// adminSessionView reports the active round: phase, deadline, and who the round
// is still waiting on.
func (b *Bot) adminSessionView() adminView {
	session, err := b.sessionRepository.GetActiveSession(context.Background())
	if err != nil {
		log.Printf("admin: cannot load active session: %v", err)
		return b.adminErrorView()
	}
	if session == nil {
		return adminView{text: b.messages.AdminNoActiveSession, keyboard: backKeyboard(b.messages.AdminBtnBack)}
	}

	lines := []string{fmt.Sprintf(b.messages.AdminSessionHeader, session.Name, b.phaseLabel(session.Status))}
	now := time.Now().UTC()

	switch session.Status {
	case models.StatusGathering:
		lines = append(lines, b.deadlineLine(session.Gathering.Deadline, now), "")

		var submitted, skipped, pending []string
		for _, p := range session.Gathering.Participants {
			name := elide(displayName(p), adminNameMaxLen)
			switch p.Step {
			case models.StepDone:
				submitted = append(submitted, name)
			case models.StepSkipped:
				skipped = append(skipped, name)
			default:
				pending = append(pending, name)
			}
		}
		lines = append(lines,
			fmt.Sprintf(b.messages.AdminSessionSubmitted, len(submitted), b.nameList(submitted)),
			fmt.Sprintf(b.messages.AdminSessionPending, len(pending), b.nameList(pending)),
			fmt.Sprintf(b.messages.AdminSessionSkipped, len(skipped), b.nameList(skipped)),
		)
	case models.StatusVoting:
		if session.Voting == nil {
			// The poll never made it out (see wedgedVotingGrace in recovery.go).
			// This is exactly the state an admin opens the console to diagnose, so
			// say so rather than showing a bare header.
			lines = append(lines, b.messages.AdminSessionNoPoll)
			break
		}
		lines = append(lines,
			b.deadlineLine(session.Voting.Deadline, now),
			"",
			fmt.Sprintf(b.messages.AdminSessionVotes, len(session.Voting.VoterIDs), session.Voting.TotalParticipants),
		)
	case models.StatusReading:
		if session.Reading != nil {
			lines = append(lines, fmt.Sprintf(b.messages.AdminSessionReadingBook,
				elide(b.bookLabel(session.Reading.Book.Title, session.Reading.Book.Author), adminNameMaxLen*2)))
		}
	}

	return adminView{text: panelText(lines), keyboard: backKeyboard(b.messages.AdminBtnBack)}
}

// adminEndConfirmView asks before ending the round. Ending is irreversible, so
// it never happens on the first press.
func (b *Bot) adminEndConfirmView() adminView {
	session, err := b.sessionRepository.GetActiveSession(context.Background())
	if err != nil {
		log.Printf("admin: cannot load active session: %v", err)
		return b.adminErrorView()
	}
	if session == nil {
		return adminView{text: b.messages.AdminNoActiveSession, keyboard: backKeyboard(b.messages.AdminBtnBack)}
	}

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.messages.AdminBtnEndConfirm, callbackAdminEndConfirm),
		),
		backRow(b.messages.AdminBtnBack),
	)
	text := fmt.Sprintf(b.messages.AdminEndConfirm, session.Name, b.phaseLabel(session.Status))
	return adminView{text: text, keyboard: &kb}
}

// endActiveSession cancels the round on the admin's behalf.
//
// The group poll is closed BEFORE the status becomes terminal, for the same
// reason closeTelegramPoll completes only after its own StopPoll succeeds: once
// the session is cancelled it is no longer active, so nothing — not the
// recovery loop, not a later vote — would ever retry the stop, and the group
// would be left voting in a poll no one counts. A failed stop therefore leaves
// the round alone so the admin can press again.
//
// The status write itself goes through b.mu, the lock every other phase
// transition takes, so it cannot race the recovery loop.
func (b *Bot) endActiveSession() (adminView, string) {
	b.mu.Lock()
	session, err := b.sessionRepository.GetActiveSession(context.Background())
	b.mu.Unlock()
	if err != nil {
		log.Printf("admin: cannot load active session to end it: %v", err)
		return b.adminErrorView(), b.messages.AdminActionFailed
	}
	if session == nil {
		// Someone (or the deadline) got there first — nothing to end.
		return b.adminRootView(), ""
	}

	if pollID, ok := openPoll(session); ok && b.cfg.GroupId != 0 {
		if err := b.stopPoll(pollID); err != nil {
			log.Printf("admin: cannot stop the poll of session %s, leaving the round alone: %v", session.ID.Hex(), err)
			return b.adminEndConfirmView(), b.messages.AdminEndPollFailed
		}
	}

	b.mu.Lock()
	current, err := b.sessionRepository.GetActiveSession(context.Background())
	if err != nil {
		b.mu.Unlock()
		log.Printf("admin: cannot re-read the session to end it: %v", err)
		return b.adminErrorView(), b.messages.AdminActionFailed
	}
	if current == nil {
		b.mu.Unlock()
		return b.adminRootView(), ""
	}
	if current.ID != session.ID {
		// A different round started while the poll was being closed; cancelling it
		// is not what the admin confirmed.
		b.mu.Unlock()
		return b.adminRootView(), ""
	}
	if err := b.sessionRepository.SetStatus(context.Background(), current.ID, models.StatusCancelled); err != nil {
		b.mu.Unlock()
		log.Printf("admin: cannot cancel session %s: %v", current.ID.Hex(), err)
		return b.adminErrorView(), b.messages.AdminActionFailed
	}
	b.mu.Unlock()

	log.Printf("admin: session %s cancelled manually from the console", current.ID.Hex())

	// A poll that appeared between the two reads (the round advanced to voting
	// under us) still has to come down, but the round is already cancelled by
	// now, so this one is best effort.
	if pollID, ok := openPoll(current); ok && b.cfg.GroupId != 0 && !samePoll(session, pollID) {
		if err := b.stopPoll(pollID); err != nil {
			log.Printf("admin: cannot stop the poll started under the cancelled session %s: %v", current.ID.Hex(), err)
		}
	}

	if b.cfg.GroupId != 0 {
		b.sendMessage(b.cfg.GroupId, b.messages.AdminRoundCancelledGroup)
	}

	return b.adminRootView(), b.messages.AdminEndDone
}

// openPoll returns the message id of the session's group poll, if it has one.
func openPoll(session *models.BookClubSession) (int, bool) {
	if session.Status != models.StatusVoting || session.Voting == nil {
		return 0, false
	}
	return session.Voting.TelegramPollID, true
}

// samePoll reports whether the session already carried this poll, so an
// already-stopped poll is not stopped twice.
func samePoll(session *models.BookClubSession, pollID int) bool {
	return session.Voting != nil && session.Voting.TelegramPollID == pollID
}

// adminUnsubListView offers the active subscribers as buttons, one per row.
func (b *Bot) adminUnsubListView(page int) adminView {
	subs, err := b.activeSubscribers()
	if err != nil {
		return b.adminErrorView()
	}
	if len(subs) == 0 {
		return adminView{text: b.messages.AdminMembersEmpty, keyboard: backKeyboard(b.messages.AdminBtnBack)}
	}

	start, end, pages, page := paginate(len(subs), page, adminPageSize)

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, end-start+2)
	for _, s := range subs[start:end] {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				memberLabel(s, adminButtonNameMaxLen),
				callbackAdminUnsubAsk+strconv.FormatInt(s.ID, 10),
			),
		))
	}
	rows = append(rows, b.pagerRows(callbackAdminUnsubPage, page, pages)...)
	rows = append(rows, backRow(b.messages.AdminBtnBack))

	text := b.messages.AdminUnsubTitle
	if pages > 1 {
		text += "\n\n" + fmt.Sprintf(b.messages.AdminMembersPage, page+1, pages)
	}
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	return adminView{text: text, keyboard: &kb}
}

// adminUnsubConfirmView asks before archiving a member.
func (b *Bot) adminUnsubConfirmView(id int64) adminView {
	s, err := b.subRepository.GetSubscriberById(context.Background(), id)
	if err != nil {
		log.Printf("admin: cannot load subscriber %d: %v", id, err)
		return b.adminErrorView()
	}
	if s == nil || s.Archived {
		return prefixNotice(b.adminUnsubListView(0), b.messages.AdminUnsubGone)
	}

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				b.messages.AdminBtnUnsubConfirm,
				callbackAdminUnsubConfirm+strconv.FormatInt(id, 10),
			),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(b.messages.AdminBtnBack, callbackAdminUnsubPage+"0"),
		),
	)
	text := fmt.Sprintf(b.messages.AdminUnsubConfirm, memberLabel(s, adminNameMaxLen))
	return adminView{text: text, keyboard: &kb}
}

// unsubscribeMember archives a member on the admin's behalf, then returns to the
// list so several members can be handled in a row.
func (b *Bot) unsubscribeMember(id int64) (adminView, string) {
	s, err := b.subRepository.GetSubscriberById(context.Background(), id)
	if err != nil {
		log.Printf("admin: cannot load subscriber %d: %v", id, err)
		return b.adminErrorView(), b.messages.AdminActionFailed
	}
	if s == nil || s.Archived {
		return b.adminUnsubListView(0), b.messages.AdminUnsubGone
	}
	if err := b.subRepository.SetArchiveSubscriber(context.Background(), id, true); err != nil {
		log.Printf("admin: cannot archive subscriber %d: %v", id, err)
		return b.adminErrorView(), b.messages.AdminActionFailed
	}

	log.Printf("admin: subscriber %d unsubscribed from the console", id)
	b.dropFromGathering(id)

	toast := fmt.Sprintf(b.messages.AdminUnsubDone, memberLabel(s, adminNameMaxLen))
	return b.adminUnsubListView(0), toast
}

// dropFromGathering releases a round in flight from a member who has just been
// unsubscribed. Archiving alone only stops future invitations: the member stays
// in the current session's participant list, where pendingParticipants keeps
// @mentioning them in every group reminder and DMing them the last call, and
// where allBooksChosen keeps waiting for a submission that will never come.
//
// A member who already submitted is left as they are — their book was offered
// to the club and is on the ballot; quietly shrinking the ballot mid-round is a
// bigger surprise than a book from someone who has since left.
func (b *Bot) dropFromGathering(id int64) {
	session, err := b.sessionRepository.GetActiveSession(context.Background())
	if err != nil {
		log.Printf("admin: cannot check the active round for unsubscribed member %d: %v", id, err)
		return
	}
	if session == nil || session.Status != models.StatusGathering {
		return
	}
	p := findParticipant(session, id)
	if p == nil || p.Step == models.StepDone || p.Step == models.StepSkipped {
		return
	}

	p.Step = models.StepSkipped
	b.persistParticipant(session.ID, p)
	log.Printf("admin: unsubscribed member %d dropped from round %s", id, session.ID.Hex())

	// They may have been the last one the round was waiting for.
	if allBooksChosen(session) {
		b.runTelegramPollFlow()
	}
}

// activeSubscribers loads the club's members in a stable order, so page 2 of a
// list means the same thing on every render.
func (b *Bot) activeSubscribers() ([]*models.Subscriber, error) {
	subs, err := b.subRepository.GetAllSubscribers(context.Background())
	if err != nil {
		log.Printf("admin: cannot load subscribers: %v", err)
		return nil, err
	}
	sort.SliceStable(subs, func(i, j int) bool {
		left, right := subscriberName(subs[i]), subscriberName(subs[j])
		if left != right {
			return left < right
		}
		return subs[i].ID < subs[j].ID
	})
	return subs, nil
}

// editPanel rewrites the panel in place, which is what makes the console feel
// like one screen instead of a stream of messages.
func (b *Bot) editPanel(cq *tgbotapi.CallbackQuery, view adminView) {
	if cq.Message == nil {
		return
	}
	edit := tgbotapi.NewEditMessageText(cq.Message.Chat.ID, cq.Message.MessageID, view.text)
	edit.ReplyMarkup = view.keyboard
	if _, err := b.tgBot.Request(edit); err != nil {
		if isNotModified(err) {
			// Pressing the button of the view already on screen: Telegram rejects
			// an edit that changes nothing, and there is nothing to change.
			return
		}
		log.Printf("admin: cannot update the panel on message %d: %v", cq.Message.MessageID, err)
	}
}

// isNotModified reports whether an edit failed only because the message already
// holds exactly the content it was asked for.
func isNotModified(err error) bool {
	return err != nil && strings.Contains(err.Error(), "message is not modified")
}

// prefixNotice puts a one-line notice above a view, for when the thing the
// admin pressed on has disappeared under them.
func prefixNotice(view adminView, notice string) adminView {
	view.text = notice + "\n\n" + view.text
	return view
}

// adminErrorView is the screen shown when the console cannot read what it needs.
func (b *Bot) adminErrorView() adminView {
	return adminView{text: b.messages.AdminActionFailed, keyboard: backKeyboard(b.messages.AdminBtnBack)}
}

// pagerRows builds the previous/next row for a paged list, or nothing when the
// list fits on one page.
func (b *Bot) pagerRows(prefix string, page, pages int) [][]tgbotapi.InlineKeyboardButton {
	if pages <= 1 {
		return nil
	}
	var row []tgbotapi.InlineKeyboardButton
	if page > 0 {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(b.messages.AdminBtnPrev, prefix+strconv.Itoa(page-1)))
	}
	if page < pages-1 {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(b.messages.AdminBtnNext, prefix+strconv.Itoa(page+1)))
	}
	if len(row) == 0 {
		return nil
	}
	return [][]tgbotapi.InlineKeyboardButton{row}
}

// backRow is the row returning to the console's home screen.
func backRow(label string) []tgbotapi.InlineKeyboardButton {
	return tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(label, callbackAdminRoot))
}

func backKeyboard(label string) *tgbotapi.InlineKeyboardMarkup {
	kb := tgbotapi.NewInlineKeyboardMarkup(backRow(label))
	return &kb
}

// deadlineLine renders a deadline as an absolute time plus how long is left,
// in the configured timezone when there is one.
func (b *Bot) deadlineLine(deadline, now time.Time) string {
	loc := b.cfg.Location
	if loc == nil {
		loc = time.UTC
	}
	stamp := deadline.In(loc).Format("02.01 15:04 MST")
	if remaining := deadline.Sub(now); remaining > 0 {
		return fmt.Sprintf(b.messages.AdminSessionDeadline, stamp, b.formatRemaining(remaining))
	}
	return fmt.Sprintf(b.messages.AdminSessionDeadlinePast, stamp)
}

// nameList renders a group of members for the session view, or a placeholder
// when the group is empty.
func (b *Bot) nameList(names []string) string {
	if len(names) == 0 {
		return b.messages.AdminNobody
	}
	return strings.Join(names, ", ")
}

// phaseLabel is the localized name of a session status.
func (b *Bot) phaseLabel(status string) string {
	switch status {
	case models.StatusGathering:
		return b.messages.AdminPhaseGathering
	case models.StatusVoting:
		return b.messages.AdminPhaseVoting
	case models.StatusReading:
		return b.messages.AdminPhaseReading
	}
	return status
}

// memberLabel is how a member is named in the console: their name, plus their
// @nick when they have one, bounded to limit.
func memberLabel(s *models.Subscriber, limit int) string {
	label := subscriberName(s)
	if s.Nick != "" {
		label += " (@" + s.Nick + ")"
	}
	return elide(label, limit)
}

// subscriberName is a subscriber's display name, falling back to their id so a
// label is never empty.
func subscriberName(s *models.Subscriber) string {
	name := strings.TrimSpace(s.FirstName + " " + s.LastName)
	if name == "" {
		return strconv.FormatInt(s.ID, 10)
	}
	return name
}

// panelText joins the panel's lines and bounds the result. The panel embeds
// member-supplied names, and a message over the limit is not truncated by
// Telegram — the whole send fails.
func panelText(lines []string) string {
	return elide(strings.Join(lines, "\n"), telegramMessageMaxLen)
}

// paginate clamps page into range and returns the slice bounds for it, along
// with the page count and the page actually used. A list that shrank under a
// stale button lands on the last page rather than out of bounds.
func paginate(total, page, size int) (start, end, pages, current int) {
	if size < 1 {
		size = 1
	}
	pages = (total + size - 1) / size
	if pages < 1 {
		pages = 1
	}
	current = page
	if current < 0 {
		current = 0
	}
	if current > pages-1 {
		current = pages - 1
	}
	start = current * size
	end = start + size
	if end > total {
		end = total
	}
	return start, end, pages, current
}

// pageArg reads the page number a list button carries. Anything unparsable is
// page one: a malformed payload should show the list, not an error.
func pageArg(data, prefix string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(data, prefix))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// idArg reads the subscriber id a member button carries.
func idArg(data, prefix string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimPrefix(data, prefix), 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
