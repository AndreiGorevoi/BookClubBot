package bot

import (
	"BookClubBot/config"
	"BookClubBot/message"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTelegram is an httptest server standing in for the Bot API. It answers
// every method with an empty successful result and records the text of each
// sendMessage call, so a test can assert on what the bot said.
type fakeTelegram struct {
	srv  *httptest.Server
	mu   sync.Mutex
	sent []sentMessage
}

type sentMessage struct {
	chatID string
	text   string
}

func newFakeTelegram(t *testing.T) *fakeTelegram {
	t.Helper()
	f := &fakeTelegram{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		if r.URL.Path == "/bottest-token/sendMessage" {
			f.mu.Lock()
			f.sent = append(f.sent, sentMessage{chatID: r.Form.Get("chat_id"), text: r.Form.Get("text")})
			f.mu.Unlock()
		}
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
