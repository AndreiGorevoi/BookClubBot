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
	statusSet     []string
	gatherNotify  int
	votingNotify  int
	votingClosed  int
	startedVoting int
}

func (f *fakeSessionRepo) CreateSession(context.Context, *models.BookClubSession) error {
	return nil
}
func (f *fakeSessionRepo) GetActiveSession(context.Context) (*models.BookClubSession, error) {
	return nil, nil
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
	return nil
}
func (f *fakeSessionRepo) SetGatheringNotified(context.Context, primitive.ObjectID, time.Time) error {
	f.gatherNotify++
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

func TestRecoverVotingCancelsWedgedSession(t *testing.T) {
	fake := &fakeSessionRepo{}
	b := &Bot{sessionRepository: fake}

	// status=voting but no voting sub-document: the poll never started.
	session := &models.BookClubSession{
		ID:     primitive.NewObjectID(),
		Status: models.StatusVoting,
		Voting: nil,
	}

	b.recoverVoting(session, time.Now().UTC())

	assert.Equal(t, []string{models.StatusCancelled}, fake.statusSet,
		"a voting session with no voting sub-document should be cancelled")
	assert.Equal(t, 0, fake.votingClosed, "wedged session must not be closed")
}
