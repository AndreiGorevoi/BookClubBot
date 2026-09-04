package bot

import (
	"BookClubBot/internal/models"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// fakeSessionRepo records the calls the recovery loop makes. Methods not needed
// by a given test are no-ops.
type fakeSessionRepo struct {
	// active is what GetActiveSession returns; nil means no round is in flight.
	active        *models.BookClubSession
	activeErr     error
	statusSet     []string
	gatherNotify  int
	votingNotify  int
	votingClosed  int
	startedVoting int
	remindersSent int
	remindersErr  error
	created       int
}

func (f *fakeSessionRepo) CreateSession(context.Context, *models.BookClubSession) error {
	f.created++
	return nil
}
func (f *fakeSessionRepo) GetActiveSession(context.Context) (*models.BookClubSession, error) {
	return f.active, f.activeErr
}
func (f *fakeSessionRepo) UpdateParticipant(context.Context, primitive.ObjectID, *models.Participant) error {
	return nil
}
func (f *fakeSessionRepo) AddVoter(context.Context, primitive.ObjectID, int64) error { return nil }
func (f *fakeSessionRepo) StartVoting(context.Context, primitive.ObjectID, *models.Voting) error {
	f.startedVoting++
	return nil
}
func (f *fakeSessionRepo) SetWinners(context.Context, primitive.ObjectID, []models.Winner) error {
	return nil
}
func (f *fakeSessionRepo) SetStatus(_ context.Context, _ primitive.ObjectID, status string) error {
	f.statusSet = append(f.statusSet, status)
	// Mirror the repository: a terminal status releases the active lock, so the
	// session stops being the active one. Without this a test could not tell a
	// cancelled round from a live one.
	if !models.IsActiveStatus(status) {
		f.active = nil
	} else if f.active != nil {
		f.active.Status = status
	}
	return nil
}
func (f *fakeSessionRepo) SetGatheringRemindersSent(_ context.Context, _ primitive.ObjectID, count int) error {
	f.gatherNotify++
	if f.remindersErr != nil {
		return f.remindersErr
	}
	f.remindersSent = count
	return nil
}
func (f *fakeSessionRepo) SetVotingNotified(context.Context, primitive.ObjectID, time.Time) error {
	f.votingNotify++
	return nil
}
func (f *fakeSessionRepo) SetVotingClosed(context.Context, primitive.ObjectID, time.Time) error {
	f.votingClosed++
	return nil
}

func TestRecoverVotingWedgedSession(t *testing.T) {
	now := time.Now().UTC()

	t.Run("recent voting==nil is mid-launch, not cancelled", func(t *testing.T) {
		fake := &fakeSessionRepo{}
		b := &Bot{sessionRepository: fake}
		session := &models.BookClubSession{
			ID:        primitive.NewObjectID(),
			Status:    models.StatusVoting,
			Voting:    nil,
			UpdatedAt: now.Add(-5 * time.Second), // within grace
		}

		b.recoverVoting(session, now)

		assert.Empty(t, fake.statusSet, "a round still launching its poll must not be cancelled")
	})

	t.Run("stale voting==nil past grace is cancelled", func(t *testing.T) {
		fake := &fakeSessionRepo{}
		b := &Bot{sessionRepository: fake}
		session := &models.BookClubSession{
			ID:        primitive.NewObjectID(),
			Status:    models.StatusVoting,
			Voting:    nil,
			UpdatedAt: now.Add(-wedgedVotingGrace - time.Second), // past grace
		}

		b.recoverVoting(session, now)

		assert.Equal(t, []string{models.StatusCancelled}, fake.statusSet,
			"a wedged voting session should be cancelled to release the lock")
		assert.Equal(t, 0, fake.votingClosed, "wedged session must not be closed")
	})
}
