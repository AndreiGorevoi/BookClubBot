package bot

import (
	"BookClubBot/config"
	"BookClubBot/internal/models"
	"BookClubBot/message"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTelegram is an httptest server standing in for the Bot API. It answers
// every method with an empty successful result and records the text of each
// sendMessage call, so a test can assert on what the bot said.
type fakeTelegram struct {
	srv   *httptest.Server
	mu    sync.Mutex
	sent  []sentMessage
	calls []apiCall
}

// apiCall is one request the bot made, kept so a test can assert on the calls
// that are not sendMessage — an edited panel, a stopped poll.
type apiCall struct {
	method string
	form   url.Values
}

type sentMessage struct {
	chatID string
	text   string
}

func newFakeTelegram(t *testing.T) *fakeTelegram {
	t.Helper()
	f := &fakeTelegram{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Runs on the server goroutine: report with t.Error, never FailNow.
		if err := r.ParseForm(); err != nil {
			t.Errorf("fake telegram: cannot parse form: %v", err)
		}
		f.mu.Lock()
		f.calls = append(f.calls, apiCall{method: strings.TrimPrefix(r.URL.Path, "/bottest-token/"), form: r.Form})
		if r.URL.Path == "/bottest-token/sendMessage" {
			f.sent = append(f.sent, sentMessage{chatID: r.Form.Get("chat_id"), text: r.Form.Get("text")})
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{}})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTelegram) api() *tgbotapi.BotAPI {
	api := &tgbotapi.BotAPI{Token: "test-token", Client: f.srv.Client()}
	api.SetAPIEndpoint(f.srv.URL + "/bot%s/%s")
	return api
}

func (f *fakeTelegram) messages() []sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentMessage(nil), f.sent...)
}

// callsTo returns every request made to one Bot API method.
func (f *fakeTelegram) callsTo(method string) []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []url.Values
	for _, c := range f.calls {
		if c.method == method {
			out = append(out, c.form)
		}
	}
	return out
}

func startVoteUpdate(from int64) *tgbotapi.Update {
	return &tgbotapi.Update{Message: &tgbotapi.Message{
		Text: "/start_vote",
		From: &tgbotapi.User{ID: from},
		Chat: &tgbotapi.Chat{ID: from},
	}}
}

func TestIsAdmin(t *testing.T) {
	t.Run("listed id is admin", func(t *testing.T) {
		b := &Bot{cfg: &config.AppConfig{AdminIDs: []int64{10, 20}}}
		assert.True(t, b.isAdmin(10))
		assert.True(t, b.isAdmin(20))
		assert.False(t, b.isAdmin(30))
	})

	t.Run("empty list denies everyone", func(t *testing.T) {
		b := &Bot{cfg: &config.AppConfig{}}
		assert.False(t, b.isAdmin(10))
		assert.False(t, b.isAdmin(0))
	})
}

func TestHandleStartVote_RejectsNonAdmin(t *testing.T) {
	tg := newFakeTelegram(t)
	sessions := &fakeSessionRepo{}
	b := &Bot{
		cfg:               &config.AppConfig{AdminIDs: []int64{1}, GroupId: -100},
		tgBot:             tg.api(),
		messages:          &message.LocalizedMessages{StartVoteAdminOnly: "admins only"},
		sessionRepository: sessions,
	}

	require.NoError(t, b.handleStartVote(startVoteUpdate(2)))

	sent := tg.messages()
	require.Len(t, sent, 1)
	assert.Equal(t, "2", sent[0].chatID)
	assert.Equal(t, "admins only", sent[0].text)
	assert.Equal(t, 0, sessions.created, "no session must be created for a non-admin")
}

func TestHandleStartVote_AdminPassesTheGate(t *testing.T) {
	// With the group id unset the handler stops right after the admin check,
	// which is enough to show that an admin is not turned away.
	tg := newFakeTelegram(t)
	sessions := &fakeSessionRepo{}
	b := &Bot{
		cfg:               &config.AppConfig{AdminIDs: []int64{1}, GroupId: 0},
		tgBot:             tg.api(),
		messages:          &message.LocalizedMessages{StartVoteAdminOnly: "admins only", CannotStartGatheringGroupIdMissing: "no group"},
		sessionRepository: sessions,
	}

	require.NoError(t, b.handleStartVote(startVoteUpdate(1)))

	sent := tg.messages()
	require.Len(t, sent, 1)
	assert.Equal(t, "no group", sent[0].text)
}

// fakeSubRepo is an in-memory subscriber store for the console's tests.
type fakeSubRepo struct {
	subs     []*models.Subscriber
	loadErr  error
	archived []int64
}

func (f *fakeSubRepo) SaveSubscriber(context.Context, *models.Subscriber) error { return nil }

func (f *fakeSubRepo) SetArchiveSubscriber(_ context.Context, id int64, archived bool) error {
	if !archived {
		return nil
	}
	f.archived = append(f.archived, id)
	for _, s := range f.subs {
		if s.ID == id {
			s.Archived = true
		}
	}
	return nil
}

func (f *fakeSubRepo) GetAllSubscribers(context.Context) ([]*models.Subscriber, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	active := make([]*models.Subscriber, 0, len(f.subs))
	for _, s := range f.subs {
		if !s.Archived {
			active = append(active, s)
		}
	}
	return active, nil
}

func (f *fakeSubRepo) GetSubscriberById(_ context.Context, id int64) (*models.Subscriber, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	for _, s := range f.subs {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, nil
}

// adminMessages fills every string the console renders with a recognisable
// stand-in, so a test can assert on what a view actually shows.
func adminMessages() *message.LocalizedMessages {
	return &message.LocalizedMessages{
		AdminOnly:                "admins only",
		AdminPanelTitle:          "panel: %s",
		AdminStatusLine:          "round %s in %s",
		AdminStatusUnknown:       "status unknown",
		AdminNoActiveSession:     "no active round",
		AdminPhaseGathering:      "gathering",
		AdminPhaseVoting:         "voting",
		AdminBtnMembers:          "Members",
		AdminBtnSession:          "Round",
		AdminBtnUnsubscribe:      "Unsubscribe",
		AdminBtnEndRound:         "End round",
		AdminBtnBack:             "Back",
		AdminBtnPrev:             "Prev",
		AdminBtnNext:             "Next",
		AdminMembersTitle:        "members: %d",
		AdminMembersEmpty:        "no members",
		AdminMembersPage:         "page %d/%d",
		AdminSessionHeader:       "round %s status %s",
		AdminSessionDeadline:     "deadline %s in %s",
		AdminSessionDeadlinePast: "deadline %s passed",
		AdminSessionSubmitted:    "submitted (%d): %s",
		AdminSessionPending:      "pending (%d): %s",
		AdminSessionSkipped:      "skipped (%d): %s",
		AdminSessionVotes:        "voted %d of %d",
		AdminNobody:              "nobody",
		AdminEndConfirm:          "end %s (%s)?",
		AdminBtnEndConfirm:       "Yes, end",
		AdminEndDone:             "round ended",
		AdminRoundCancelledGroup: "round cancelled by an admin",
		AdminUnsubTitle:          "who to unsubscribe?",
		AdminUnsubConfirm:        "unsubscribe %s?",
		AdminBtnUnsubConfirm:     "Yes, unsubscribe",
		AdminUnsubDone:           "%s is out",
		AdminUnsubGone:           "already unsubscribed",
		AdminActionFailed:        "action failed",
		TimeLeftHours:            "%d h",
		TimeLeftMinutes:          "%d min",
	}
}

func adminBot(tg *fakeTelegram, subs *fakeSubRepo, sessions *fakeSessionRepo) *Bot {
	return &Bot{
		cfg:               &config.AppConfig{AdminIDs: []int64{1}, GroupId: -100},
		tgBot:             tg.api(),
		messages:          adminMessages(),
		subRepository:     subs,
		sessionRepository: sessions,
	}
}

func adminUpdate(from int64) *tgbotapi.Update {
	return &tgbotapi.Update{Message: &tgbotapi.Message{
		Text: "/admin",
		From: &tgbotapi.User{ID: from},
		Chat: &tgbotapi.Chat{ID: from},
	}}
}

func adminCallback(from int64, data string) *tgbotapi.CallbackQuery {
	return &tgbotapi.CallbackQuery{
		ID:      "cb",
		From:    &tgbotapi.User{ID: from},
		Data:    data,
		Message: &tgbotapi.Message{MessageID: 42, Chat: &tgbotapi.Chat{ID: from}},
	}
}

func TestHandleAdmin_RejectsNonAdmin(t *testing.T) {
	tg := newFakeTelegram(t)
	b := adminBot(tg, &fakeSubRepo{}, &fakeSessionRepo{})

	require.NoError(t, b.handleAdmin(adminUpdate(2)))

	sent := tg.messages()
	require.Len(t, sent, 1)
	assert.Equal(t, "admins only", sent[0].text)
	assert.Equal(t, "2", sent[0].chatID, "the refusal goes to the caller")
}

func TestHandleAdmin_OpensPanelForAdmin(t *testing.T) {
	tg := newFakeTelegram(t)
	b := adminBot(tg, &fakeSubRepo{}, &fakeSessionRepo{})

	require.NoError(t, b.handleAdmin(adminUpdate(1)))

	sent := tg.callsTo("sendMessage")
	require.Len(t, sent, 1)
	assert.Equal(t, "panel: no active round", sent[0].Get("text"))
	assert.Contains(t, sent[0].Get("reply_markup"), callbackAdminSession)
	assert.Contains(t, sent[0].Get("reply_markup"), callbackAdminEndAsk)
}

func TestAdminCallback_DeniedForNonAdmin(t *testing.T) {
	tg := newFakeTelegram(t)
	sessions := &fakeSessionRepo{active: &models.BookClubSession{Status: models.StatusGathering}}
	b := adminBot(tg, &fakeSubRepo{}, sessions)

	b.handleAdminCallback(adminCallback(2, callbackAdminEndConfirm))

	assert.Empty(t, tg.callsTo("editMessageText"), "a non-admin must not see the panel")
	answers := tg.callsTo("answerCallbackQuery")
	require.Len(t, answers, 1)
	assert.Equal(t, "admins only", answers[0].Get("text"))
	assert.Empty(t, sessions.statusSet, "a non-admin must not end the round")
}

func TestAdminCallback_UnknownPayloadIsJustAcknowledged(t *testing.T) {
	tg := newFakeTelegram(t)
	b := adminBot(tg, &fakeSubRepo{}, &fakeSessionRepo{})

	b.handleAdminCallback(adminCallback(1, "a:whatever"))

	assert.Empty(t, tg.callsTo("editMessageText"))
	require.Len(t, tg.callsTo("answerCallbackQuery"), 1)
}

func TestAdminEndRound_CancelsAndNotifiesTheGroup(t *testing.T) {
	tg := newFakeTelegram(t)
	sessions := &fakeSessionRepo{active: &models.BookClubSession{
		Name:   "May 2026",
		Status: models.StatusGathering,
	}}
	b := adminBot(tg, &fakeSubRepo{}, sessions)

	b.handleAdminCallback(adminCallback(1, callbackAdminEndConfirm))

	assert.Equal(t, []string{models.StatusCancelled}, sessions.statusSet)
	sent := tg.messages()
	require.Len(t, sent, 1)
	assert.Equal(t, "-100", sent[0].chatID)
	assert.Equal(t, "round cancelled by an admin", sent[0].text)

	answers := tg.callsTo("answerCallbackQuery")
	require.Len(t, answers, 1)
	assert.Equal(t, "round ended", answers[0].Get("text"))
}

func TestAdminEndRound_StopsAnOpenPoll(t *testing.T) {
	tg := newFakeTelegram(t)
	sessions := &fakeSessionRepo{active: &models.BookClubSession{
		Status: models.StatusVoting,
		Voting: &models.Voting{TelegramPollID: 777},
	}}
	b := adminBot(tg, &fakeSubRepo{}, sessions)

	b.handleAdminCallback(adminCallback(1, callbackAdminEndConfirm))

	stops := tg.callsTo("stopPoll")
	require.Len(t, stops, 1)
	assert.Equal(t, "777", stops[0].Get("message_id"))
	assert.Equal(t, "-100", stops[0].Get("chat_id"))
}

func TestAdminEndRound_NoActiveRound(t *testing.T) {
	tg := newFakeTelegram(t)
	sessions := &fakeSessionRepo{}
	b := adminBot(tg, &fakeSubRepo{}, sessions)

	b.handleAdminCallback(adminCallback(1, callbackAdminEndConfirm))

	assert.Empty(t, sessions.statusSet)
	assert.Empty(t, tg.messages(), "nothing to announce when there was no round")
	edits := tg.callsTo("editMessageText")
	require.Len(t, edits, 1)
	assert.Equal(t, "panel: no active round", edits[0].Get("text"))
}

func TestAdminEndRound_ConfirmationDoesNotEndTheRound(t *testing.T) {
	tg := newFakeTelegram(t)
	sessions := &fakeSessionRepo{active: &models.BookClubSession{Name: "May 2026", Status: models.StatusGathering}}
	b := adminBot(tg, &fakeSubRepo{}, sessions)

	b.handleAdminCallback(adminCallback(1, callbackAdminEndAsk))

	assert.Empty(t, sessions.statusSet, "the round must survive the confirmation step")
	edits := tg.callsTo("editMessageText")
	require.Len(t, edits, 1)
	assert.Equal(t, "end May 2026 (gathering)?", edits[0].Get("text"))
	assert.Contains(t, edits[0].Get("reply_markup"), callbackAdminEndConfirm)
}

func TestAdminUnsubscribe_ArchivesTheMember(t *testing.T) {
	tg := newFakeTelegram(t)
	subs := &fakeSubRepo{subs: []*models.Subscriber{{ID: 7, FirstName: "Ann", Nick: "ann"}}}
	b := adminBot(tg, subs, &fakeSessionRepo{})

	b.handleAdminCallback(adminCallback(1, callbackAdminUnsubConfirm+"7"))

	assert.Equal(t, []int64{7}, subs.archived)
	answers := tg.callsTo("answerCallbackQuery")
	require.Len(t, answers, 1)
	assert.Equal(t, "Ann (@ann) is out", answers[0].Get("text"))
}

func TestAdminUnsubscribe_AlreadyArchived(t *testing.T) {
	tg := newFakeTelegram(t)
	subs := &fakeSubRepo{subs: []*models.Subscriber{{ID: 7, FirstName: "Ann", Archived: true}}}
	b := adminBot(tg, subs, &fakeSessionRepo{})

	b.handleAdminCallback(adminCallback(1, callbackAdminUnsubConfirm+"7"))

	assert.Empty(t, subs.archived, "an already archived member must not be written again")
	answers := tg.callsTo("answerCallbackQuery")
	require.Len(t, answers, 1)
	assert.Equal(t, "already unsubscribed", answers[0].Get("text"))
}

func TestAdminUnsubscribe_ConfirmationDoesNotArchive(t *testing.T) {
	tg := newFakeTelegram(t)
	subs := &fakeSubRepo{subs: []*models.Subscriber{{ID: 7, FirstName: "Ann"}}}
	b := adminBot(tg, subs, &fakeSessionRepo{})

	b.handleAdminCallback(adminCallback(1, callbackAdminUnsubAsk+"7"))

	assert.Empty(t, subs.archived)
	edits := tg.callsTo("editMessageText")
	require.Len(t, edits, 1)
	assert.Equal(t, "unsubscribe Ann?", edits[0].Get("text"))
	assert.Contains(t, edits[0].Get("reply_markup"), callbackAdminUnsubConfirm+"7")
}

func TestAdminMembersView_ListsAndPaginates(t *testing.T) {
	subs := &fakeSubRepo{}
	for i := 0; i < adminPageSize+2; i++ {
		subs.subs = append(subs.subs, &models.Subscriber{ID: int64(i + 1), FirstName: string(rune('A' + i))})
	}
	b := adminBot(newFakeTelegram(t), subs, &fakeSessionRepo{})

	first := b.adminMembersView(0)
	assert.Contains(t, first.text, "members: 10")
	assert.Contains(t, first.text, "page 1/2")
	assert.Contains(t, first.text, "1. A")

	second := b.adminMembersView(1)
	assert.Contains(t, second.text, "page 2/2")
	assert.Contains(t, second.text, "9. I")
	assert.NotContains(t, second.text, "1. A")

	// A page beyond the end (a stale button after members left) clamps back.
	assert.Equal(t, second.text, b.adminMembersView(99).text)
}

func TestAdminMembersView_ExcludesArchived(t *testing.T) {
	subs := &fakeSubRepo{subs: []*models.Subscriber{
		{ID: 1, FirstName: "Ann"},
		{ID: 2, FirstName: "Bob", Archived: true},
	}}
	b := adminBot(newFakeTelegram(t), subs, &fakeSessionRepo{})

	view := b.adminMembersView(0)
	assert.Contains(t, view.text, "Ann")
	assert.NotContains(t, view.text, "Bob")
}

func TestAdminSessionView_Gathering(t *testing.T) {
	sessions := &fakeSessionRepo{active: &models.BookClubSession{
		Name:   "May 2026",
		Status: models.StatusGathering,
		Gathering: models.Gathering{
			// A minute of slack: formatRemaining floors to whole hours, so an exact
			// two hours would already read as one by the time the view renders.
			Deadline: time.Now().UTC().Add(2*time.Hour + time.Minute),
			Participants: []*models.Participant{
				{SubscriberID: 1, FirstName: "Ann", Step: models.StepDone},
				{SubscriberID: 2, FirstName: "Bob", Step: models.StepAuthor},
				{SubscriberID: 3, FirstName: "Cid", Step: models.StepSkipped},
			},
		},
	}}
	b := adminBot(newFakeTelegram(t), &fakeSubRepo{}, sessions)

	view := b.adminSessionView()

	assert.Contains(t, view.text, "round May 2026 status gathering")
	assert.Contains(t, view.text, "submitted (1): Ann")
	assert.Contains(t, view.text, "pending (1): Bob")
	assert.Contains(t, view.text, "skipped (1): Cid")
	assert.Contains(t, view.text, "in 2 h")
}

func TestAdminSessionView_Voting(t *testing.T) {
	sessions := &fakeSessionRepo{active: &models.BookClubSession{
		Name:   "May 2026",
		Status: models.StatusVoting,
		Voting: &models.Voting{
			Deadline:          time.Now().UTC().Add(30 * time.Minute),
			VoterIDs:          []int64{1, 2},
			TotalParticipants: 5,
		},
	}}
	b := adminBot(newFakeTelegram(t), &fakeSubRepo{}, sessions)

	view := b.adminSessionView()

	assert.Contains(t, view.text, "status voting")
	assert.Contains(t, view.text, "voted 2 of 5")
}

func TestAdminSessionView_NoRound(t *testing.T) {
	b := adminBot(newFakeTelegram(t), &fakeSubRepo{}, &fakeSessionRepo{})
	assert.Equal(t, "no active round", b.adminSessionView().text)
}

func TestAllowedWithoutMembership(t *testing.T) {
	b := &Bot{cfg: &config.AppConfig{AdminIDs: []int64{1}}}

	assert.True(t, b.allowedWithoutMembership("/subscribe", 99))
	assert.True(t, b.allowedWithoutMembership("/admin", 1), "an admin need not be a club member")
	assert.False(t, b.allowedWithoutMembership("/admin", 2))
	assert.False(t, b.allowedWithoutMembership("/start_vote", 1))
}

func TestAdminCallbackPayloadsFitTelegramsLimit(t *testing.T) {
	// callback_data is capped at 64 bytes; the longest payload is a prefix plus
	// a full-width Telegram user id.
	longest := callbackAdminUnsubConfirm + "9223372036854775807"
	assert.LessOrEqual(t, len(longest), 64)
}

func TestPaginate(t *testing.T) {
	cases := map[string]struct {
		total, page, size          int
		start, end, pages, current int
	}{
		"first page":        {total: 10, page: 0, size: 4, start: 0, end: 4, pages: 3, current: 0},
		"last partial page": {total: 10, page: 2, size: 4, start: 8, end: 10, pages: 3, current: 2},
		"page past the end": {total: 10, page: 9, size: 4, start: 8, end: 10, pages: 3, current: 2},
		"negative page":     {total: 10, page: -3, size: 4, start: 0, end: 4, pages: 3, current: 0},
		"empty list":        {total: 0, page: 0, size: 4, start: 0, end: 0, pages: 1, current: 0},
		"exact fit":         {total: 8, page: 1, size: 4, start: 4, end: 8, pages: 2, current: 1},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			start, end, pages, current := paginate(tc.total, tc.page, tc.size)
			assert.Equal(t, tc.start, start)
			assert.Equal(t, tc.end, end)
			assert.Equal(t, tc.pages, pages)
			assert.Equal(t, tc.current, current)
		})
	}
}

func TestCallbackArgs(t *testing.T) {
	assert.Equal(t, 3, pageArg("a:members:3", callbackAdminMembersPage))
	assert.Equal(t, 0, pageArg("a:members:", callbackAdminMembersPage))
	assert.Equal(t, 0, pageArg("a:members:oops", callbackAdminMembersPage))
	assert.Equal(t, 0, pageArg("a:members:-2", callbackAdminMembersPage))

	id, ok := idArg("a:unsubgo:42", callbackAdminUnsubConfirm)
	assert.True(t, ok)
	assert.Equal(t, int64(42), id)

	_, ok = idArg("a:unsubgo:x", callbackAdminUnsubConfirm)
	assert.False(t, ok)
}

func TestMemberLabel(t *testing.T) {
	assert.Equal(t, "Ann Lee (@ann)", memberLabel(&models.Subscriber{FirstName: "Ann", LastName: "Lee", Nick: "ann"}, 40))
	assert.Equal(t, "Ann Lee", memberLabel(&models.Subscriber{FirstName: "Ann", LastName: "Lee"}, 40))
	assert.Equal(t, "77", memberLabel(&models.Subscriber{ID: 77}, 40), "a nameless member falls back to their id")
	assert.Equal(t, "Ann…", memberLabel(&models.Subscriber{FirstName: "Annabella"}, 4))
}

func TestIsNotModified(t *testing.T) {
	assert.True(t, isNotModified(errors.New("Bad Request: message is not modified")))
	assert.False(t, isNotModified(errors.New("Bad Request: chat not found")))
	assert.False(t, isNotModified(nil))
}
