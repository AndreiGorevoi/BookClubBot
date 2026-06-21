package repository

import (
	"BookClubBot/internal/models"
	mongo_helpers "BookClubBot/internal/repository/testing"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestNewSessionRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
	defer clear()

	repo, err := NewSessionRepository(mongoDB)
	assert.NoError(t, err)
	assert.NotNil(t, repo)

	repo, err = NewSessionRepository(nil)
	assert.Error(t, err)
	assert.Nil(t, repo)
	assert.Equal(t, ErrNilDatabase, err)
}

func TestCreateSession_EnforcesSingleActive(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
	defer cleanSession(clear, mongoDB)
	repo := newSessionRepo(t, mongoDB)

	ctx := testCtx(t)

	first := newGatheringSession(100)
	require.NoError(t, repo.CreateSession(ctx, first))
	assert.False(t, first.ID.IsZero(), "generated id should be written back")

	// A second active session must be rejected by the unique index.
	second := newGatheringSession(200)
	err := repo.CreateSession(ctx, second)
	assert.ErrorIs(t, err, ErrActiveSessionExists)

	// Once the first is completed (lock released), a new one may start.
	require.NoError(t, repo.SetStatus(ctx, first.ID, models.StatusCompleted))
	third := newGatheringSession(300)
	assert.NoError(t, repo.CreateSession(ctx, third))
}

func TestGetActiveSession(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
	defer cleanSession(clear, mongoDB)
	repo := newSessionRepo(t, mongoDB)
	ctx := testCtx(t)

	// No active session yet.
	active, err := repo.GetActiveSession(ctx)
	assert.NoError(t, err)
	assert.Nil(t, active)

	session := newGatheringSession(100)
	require.NoError(t, repo.CreateSession(ctx, session))

	active, err = repo.GetActiveSession(ctx)
	assert.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, session.ID, active.ID)
	assert.Equal(t, models.StatusGathering, active.Status)

	// Completing it makes GetActiveSession return nil again.
	require.NoError(t, repo.SetStatus(ctx, session.ID, models.StatusCompleted))
	active, err = repo.GetActiveSession(ctx)
	assert.NoError(t, err)
	assert.Nil(t, active)
}

func TestUpdateParticipant(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
	defer cleanSession(clear, mongoDB)
	repo := newSessionRepo(t, mongoDB)
	ctx := testCtx(t)

	session := newGatheringSession(100)
	require.NoError(t, repo.CreateSession(ctx, session))

	now := time.Now().UTC()
	updated := &models.Participant{
		SubscriberID: 100,
		FirstName:    "Andrei",
		Step:         models.StepDone,
		Book: &models.Book{
			Title:       "The Pragmatic Programmer",
			Author:      "David Thomas",
			Description: "A classic.",
		},
		InvitedAt:   session.Gathering.Participants[0].InvitedAt,
		SubmittedAt: &now,
	}
	require.NoError(t, repo.UpdateParticipant(ctx, session.ID, updated))

	stored, err := repo.GetSessionById(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, stored.Gathering.Participants, 1)
	p := stored.Gathering.Participants[0]
	assert.Equal(t, models.StepDone, p.Step)
	require.NotNil(t, p.Book)
	assert.Equal(t, "The Pragmatic Programmer", p.Book.Title)
	assert.Equal(t, "David Thomas", p.Book.Author)
	require.NotNil(t, p.SubmittedAt)

	// Unknown participant → ErrNotFound.
	err = repo.UpdateParticipant(ctx, session.ID, &models.Participant{SubscriberID: 999})
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestAddVoter_Idempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
	defer cleanSession(clear, mongoDB)
	repo := newSessionRepo(t, mongoDB)
	ctx := testCtx(t)

	session := newGatheringSession(100)
	require.NoError(t, repo.CreateSession(ctx, session))
	require.NoError(t, repo.StartVoting(ctx, session.ID, newVoting()))

	require.NoError(t, repo.AddVoter(ctx, session.ID, 100))
	require.NoError(t, repo.AddVoter(ctx, session.ID, 100)) // duplicate
	require.NoError(t, repo.AddVoter(ctx, session.ID, 200))

	stored, err := repo.GetSessionById(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.Voting)
	assert.ElementsMatch(t, []int64{100, 200}, stored.Voting.VoterIDs)
}

func TestStartVotingAndSetWinners(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
	defer cleanSession(clear, mongoDB)
	repo := newSessionRepo(t, mongoDB)
	ctx := testCtx(t)

	session := newGatheringSession(100)
	require.NoError(t, repo.CreateSession(ctx, session))

	require.NoError(t, repo.StartVoting(ctx, session.ID, newVoting()))
	stored, err := repo.GetSessionById(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, models.StatusVoting, stored.Status)
	require.NotNil(t, stored.Voting)
	assert.Equal(t, 42, stored.Voting.TelegramPollID)

	// Session is still active during voting.
	active, err := repo.GetActiveSession(ctx)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, session.ID, active.ID)

	winners := []models.Winner{{SubscriberID: 100, Title: "The Pragmatic Programmer", Author: "David Thomas"}}
	require.NoError(t, repo.SetWinners(ctx, session.ID, winners))
	stored, err = repo.GetSessionById(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, stored.Winners, 1)
	assert.Equal(t, "The Pragmatic Programmer", stored.Winners[0].Title)
}

func TestSetGatheringAndVotingNotified(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
	defer cleanSession(clear, mongoDB)
	repo := newSessionRepo(t, mongoDB)
	ctx := testCtx(t)

	session := newGatheringSession(100)
	require.NoError(t, repo.CreateSession(ctx, session))
	require.NoError(t, repo.StartVoting(ctx, session.ID, newVoting()))

	at := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.SetGatheringNotified(ctx, session.ID, at))
	require.NoError(t, repo.SetVotingNotified(ctx, session.ID, at))

	stored, err := repo.GetSessionById(ctx, session.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.Gathering.NotifiedAt)
	require.NotNil(t, stored.Voting.NotifiedAt)
	assert.Equal(t, at, stored.Gathering.NotifiedAt.UTC())
	assert.Equal(t, at, stored.Voting.NotifiedAt.UTC())
}

func TestListPastSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	mongoDB, clear := mongo_helpers.CreateTestMongoDB(t)
	defer cleanSession(clear, mongoDB)
	repo := newSessionRepo(t, mongoDB)
	ctx := testCtx(t)

	// Create and complete two sessions in sequence (one active at a time).
	older := newGatheringSession(100)
	older.Name = "May 2026"
	require.NoError(t, repo.CreateSession(ctx, older))
	require.NoError(t, repo.SetWinners(ctx, older.ID, []models.Winner{{SubscriberID: 100, Title: "Older"}}))
	require.NoError(t, repo.SetStatus(ctx, older.ID, models.StatusCompleted))

	newer := newGatheringSession(200)
	newer.Name = "June 2026"
	require.NoError(t, repo.CreateSession(ctx, newer))
	require.NoError(t, repo.SetStatus(ctx, newer.ID, models.StatusCompleted))

	// An active session must be excluded from history.
	active := newGatheringSession(300)
	require.NoError(t, repo.CreateSession(ctx, active))

	past, err := repo.ListPastSessions(ctx, 0)
	require.NoError(t, err)
	require.Len(t, past, 2)
	assert.Equal(t, "June 2026", past[0].Name, "newest first")
	assert.Equal(t, "May 2026", past[1].Name)

	limited, err := repo.ListPastSessions(ctx, 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
	assert.Equal(t, "June 2026", limited[0].Name)
}

// --- helpers ---

func newSessionRepo(t *testing.T, db *mongo.Database) *SessionRepository {
	t.Helper()
	repo, err := NewSessionRepository(db)
	require.NoError(t, err)
	require.NoError(t, repo.EnsureIndexes(testCtx(t)))
	return repo
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func newGatheringSession(subscriberID int64) *models.BookClubSession {
	now := time.Now().UTC()
	return &models.BookClubSession{
		Name:      "June 2026",
		Status:    models.StatusGathering,
		CreatedBy: subscriberID,
		Gathering: models.Gathering{
			Deadline: now.Add(48 * time.Hour),
			NotifyAt: now.Add(46 * time.Hour),
			Participants: []*models.Participant{
				{
					SubscriberID: subscriberID,
					FirstName:    "Test",
					Step:         models.StepBook,
					InvitedAt:    now,
				},
			},
		},
	}
}

func newVoting() *models.Voting {
	now := time.Now().UTC()
	return &models.Voting{
		TelegramPollID:    42,
		Deadline:          now.Add(24 * time.Hour),
		NotifyAt:          now.Add(22 * time.Hour),
		TotalParticipants: 2,
		StartedAt:         now,
	}
}

func cleanSession(clear func(), mongoDB *mongo.Database) {
	clear()
	mongo_helpers.DropCollection(mongoDB, sessions_collection)
}
