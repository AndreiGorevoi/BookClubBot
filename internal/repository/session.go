package repository

import (
	"BookClubBot/internal/models"
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const sessions_collection = "book_club_sessions"

type SessionRepository struct {
	db *mongo.Database
}

func NewSessionRepository(db *mongo.Database) (*SessionRepository, error) {
	if db == nil {
		return nil, ErrNilDatabase
	}
	return &SessionRepository{
		db: db,
	}, nil
}

// EnsureIndexes creates the indexes the session collection relies on:
//   - a unique partial index on activeLock that allows at most one active
//     session at a time (only active sessions carry the field);
//   - a descending createdAt index for history listing.
//
// MongoDB 6.0 does not support $in inside partialFilterExpression, so the
// "one active session" rule is expressed via the existence of activeLock
// rather than a status set.
func (s *SessionRepository) EnsureIndexes(ctx context.Context) error {
	collection := s.db.Collection(sessions_collection)
	_, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "activeLock", Value: 1}},
			Options: options.Index().
				SetName("uniq_active_session").
				SetUnique(true).
				SetPartialFilterExpression(bson.M{"activeLock": bson.M{"$exists": true}}),
		},
		{
			Keys:    bson.D{{Key: "createdAt", Value: -1}},
			Options: options.Index().SetName("createdAt_desc"),
		},
	})
	return err
}

// CreateSession inserts a new session. createdAt/updatedAt are stamped here, and
// activeLock is set when the session starts in an active status so the unique
// index is enforced. The generated ID is written back onto the session.
// Returns ErrActiveSessionExists if another active session already exists.
func (s *SessionRepository) CreateSession(ctx context.Context, session *models.BookClubSession) error {
	now := time.Now().UTC()
	session.CreatedAt = now
	session.UpdatedAt = now
	session.ActiveLock = activeLock(session.IsActive())

	collection := s.db.Collection(sessions_collection)
	res, err := collection.InsertOne(ctx, session)
	if err != nil {
		return mapActiveLockConflict(err)
	}
	if oid, ok := res.InsertedID.(primitive.ObjectID); ok {
		session.ID = oid
	}
	return nil
}

// GetActiveSession returns the single active session, or (nil, nil) if there is
// none.
func (s *SessionRepository) GetActiveSession(ctx context.Context) (*models.BookClubSession, error) {
	collection := s.db.Collection(sessions_collection)
	filter := bson.M{"activeLock": bson.M{"$exists": true}}

	var session models.BookClubSession
	if err := collection.FindOne(ctx, filter).Decode(&session); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

// GetSessionById returns the session with the given id, or (nil, nil) if none.
func (s *SessionRepository) GetSessionById(ctx context.Context, id primitive.ObjectID) (*models.BookClubSession, error) {
	collection := s.db.Collection(sessions_collection)

	var session models.BookClubSession
	if err := collection.FindOne(ctx, bson.M{"_id": id}).Decode(&session); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

// UpdateParticipant replaces the gathering participant matching
// participant.SubscriberID. Returns ErrNotFound if no such participant exists.
func (s *SessionRepository) UpdateParticipant(ctx context.Context, id primitive.ObjectID, participant *models.Participant) error {
	collection := s.db.Collection(sessions_collection)
	filter := bson.M{
		"_id":                                 id,
		"gathering.participants.subscriberId": participant.SubscriberID,
	}
	update := bson.M{"$set": bson.M{
		"gathering.participants.$": participant,
		"updatedAt":                time.Now().UTC(),
	}}

	res, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// AddVoter records that a subscriber has voted (idempotent via $addToSet).
// It requires voting to have started; if the session has no voting sub-document
// yet, it returns ErrNotFound rather than a raw "$addToSet on null" write error.
func (s *SessionRepository) AddVoter(ctx context.Context, id primitive.ObjectID, voterID int64) error {
	collection := s.db.Collection(sessions_collection)
	filter := bson.M{"_id": id, "voting": bson.M{"$ne": nil}}
	update := bson.M{
		"$addToSet": bson.M{"voting.voterIds": voterID},
		"$set":      bson.M{"updatedAt": time.Now().UTC()},
	}

	res, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// StartVoting attaches the voting sub-document and moves the session into the
// voting status. It (re)asserts activeLock so the "active status ⟺ activeLock
// present" invariant holds locally, rather than relying on the lock already
// being there; setting it on the same document is a no-op, and the unique index
// surfaces ErrActiveSessionExists if another session is somehow active.
func (s *SessionRepository) StartVoting(ctx context.Context, id primitive.ObjectID, voting *models.Voting) error {
	// Store an empty array (not BSON null) so later $addToSet on voterIds works.
	if voting.VoterIDs == nil {
		voting.VoterIDs = []int64{}
	}

	collection := s.db.Collection(sessions_collection)
	filter := bson.M{"_id": id}
	update := bson.M{"$set": bson.M{
		"voting":     voting,
		"status":     models.StatusVoting,
		"activeLock": true,
		"updatedAt":  time.Now().UTC(),
	}}

	res, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return mapActiveLockConflict(err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// SetWinners stores the winning book(s) of a session.
func (s *SessionRepository) SetWinners(ctx context.Context, id primitive.ObjectID, winners []models.Winner) error {
	collection := s.db.Collection(sessions_collection)
	filter := bson.M{"_id": id}
	update := bson.M{"$set": bson.M{
		"winners":   winners,
		"updatedAt": time.Now().UTC(),
	}}

	res, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// SetStatus transitions a session to a new status. Moving to a terminal status
// (completed/cancelled) releases the active lock so a new session can start;
// moving to an active status (re)asserts it.
func (s *SessionRepository) SetStatus(ctx context.Context, id primitive.ObjectID, status string) error {
	collection := s.db.Collection(sessions_collection)
	filter := bson.M{"_id": id}

	set := bson.M{"status": status, "updatedAt": time.Now().UTC()}
	update := bson.M{"$set": set}
	if models.IsActiveStatus(status) {
		set["activeLock"] = true
	} else {
		update["$unset"] = bson.M{"activeLock": ""}
	}

	res, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return mapActiveLockConflict(err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// SetGatheringNotified marks the gathering pre-deadline reminder as sent.
func (s *SessionRepository) SetGatheringNotified(ctx context.Context, id primitive.ObjectID, at time.Time) error {
	return s.setField(ctx, id, "gathering.notifiedAt", at.UTC())
}

// SetVotingNotified marks the voting pre-deadline reminder as sent.
func (s *SessionRepository) SetVotingNotified(ctx context.Context, id primitive.ObjectID, at time.Time) error {
	return s.setField(ctx, id, "voting.notifiedAt", at.UTC())
}

// SetVotingClosed stamps the time the poll was closed.
func (s *SessionRepository) SetVotingClosed(ctx context.Context, id primitive.ObjectID, at time.Time) error {
	return s.setField(ctx, id, "voting.closedAt", at.UTC())
}

// ListPastSessions returns completed sessions, newest first, up to limit
// (limit <= 0 means no limit).
func (s *SessionRepository) ListPastSessions(ctx context.Context, limit int64) ([]*models.BookClubSession, error) {
	collection := s.db.Collection(sessions_collection)
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}})
	if limit > 0 {
		opts.SetLimit(limit)
	}

	cursor, err := collection.Find(ctx, bson.M{"status": models.StatusCompleted}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var sessions []*models.BookClubSession
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (s *SessionRepository) setField(ctx context.Context, id primitive.ObjectID, field string, value any) error {
	collection := s.db.Collection(sessions_collection)
	update := bson.M{"$set": bson.M{
		field:       value,
		"updatedAt": time.Now().UTC(),
	}}

	res, err := collection.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// mapActiveLockConflict translates a duplicate-key error from the unique
// activeLock index into ErrActiveSessionExists, so every write that could
// collide with an existing active session reports the condition the same way.
func mapActiveLockConflict(err error) error {
	if mongo.IsDuplicateKeyError(err) {
		return ErrActiveSessionExists
	}
	return err
}

// activeLock returns a pointer to true when active, or nil so the field is
// omitted from the document (omitempty) and excluded from the unique index.
func activeLock(active bool) *bool {
	if !active {
		return nil
	}
	t := true
	return &t
}
