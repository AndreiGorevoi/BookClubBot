package bot

import (
	"BookClubBot/internal/models"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gatheringSession is a round in gathering whose n participants all finished,
// participant i owning the book whose photo file id is "photo-<i>".
func gatheringSession(n int) *models.BookClubSession {
	session := &models.BookClubSession{Status: models.StatusGathering}
	for i := 1; i <= n; i++ {
		session.Gathering.Participants = append(session.Gathering.Participants, &models.Participant{
			SubscriberID: int64(i),
			Step:         models.StepDone,
			Book: &models.Book{
				Title:   fmt.Sprintf("Book %d", i),
				Author:  fmt.Sprintf("Author %d", i),
				PhotoID: fmt.Sprintf("photo-%d", i),
			},
		})
	}
	return session
}

// albumOwners decodes the sent media groups back into the owners of their
// photos, in the order the group saw them.
func albumOwners(t *testing.T, tg *fakeTelegram) []int64 {
	t.Helper()
	var owners []int64
	for _, form := range tg.callsTo("sendMediaGroup") {
		var media []struct {
			Media string `json:"media"`
		}
		require.NoError(t, json.Unmarshal([]byte(form.Get("media")), &media))
		for _, m := range media {
			var id int64
			_, err := fmt.Sscanf(m.Media, "photo-%d", &id)
			require.NoError(t, err, "unexpected media %q", m.Media)
			owners = append(owners, id)
		}
	}
	return owners
}

// runRound drives a gathering session of n books into voting and returns the
// album order and the poll's option owners.
func runRound(t *testing.T, n int) (album, poll []int64) {
	t.Helper()
	tg := newFakeTelegram(t)
	sessions := &fakeSessionRepo{active: gatheringSession(n)}
	b := adminBot(tg, &fakeSubRepo{}, sessions)

	b.runTelegramPollFlow()

	require.NotNil(t, sessions.voting, "the poll should have started")
	return albumOwners(t, tg), sessions.voting.OptionOwners
}

func TestBallotOrder_AlbumMatchesPoll(t *testing.T) {
	album, poll := runRound(t, 6)
	assert.Len(t, poll, 6)
	assert.Equal(t, poll, album, "the album must list the books in the poll's order")
}

func TestBallotOrder_AlbumIsShuffled(t *testing.T) {
	stored := []int64{1, 2, 3, 4, 5, 6}
	// A single round can land on the stored order by chance (1 in 720); twenty
	// in a row all doing so means the album is not shuffled at all.
	for i := 0; i < 20; i++ {
		album, poll := runRound(t, 6)
		require.Equal(t, poll, album)
		if !assert.ObjectsAreEqual(stored, album) {
			return
		}
	}
	t.Fatal("the album always followed the stored participant order")
}

func TestBallotOrder_SkipsUnfinished(t *testing.T) {
	session := gatheringSession(3)
	session.Gathering.Participants[1].Step = models.StepSkipped
	session.Gathering.Participants[2].Step = models.StepImage

	ballot := ballotOrder(session)
	require.Len(t, ballot, 1)
	assert.Equal(t, int64(1), ballot[0].SubscriberID)
}

func TestBallotOrder_CapsAlbumAndPollTogether(t *testing.T) {
	album, poll := runRound(t, 12)
	assert.Len(t, poll, maxBallotSize)
	assert.Equal(t, poll, album, "every book in the album must be votable, and vice versa")
}

func TestBallotOrder_CapIsRandom(t *testing.T) {
	// Cutting before the shuffle would always drop books 11 and 12.
	for i := 0; i < 20; i++ {
		ballot := ballotOrder(gatheringSession(12))
		require.Len(t, ballot, maxBallotSize)
		for _, p := range ballot {
			if p.SubscriberID > maxBallotSize {
				return
			}
		}
	}
	t.Fatal("the books past the cap never made the ballot")
}
