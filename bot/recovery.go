package bot

import (
	"BookClubBot/internal/models"
	"context"
	"log"
	"time"
)

// recoveryTickInterval is how often the recovery loop re-evaluates the active
// session. A deadline may therefore fire up to one interval late, which is
// irrelevant for a book club measured in days. See docs/book-club-flow.md.
const recoveryTickInterval = 15 * time.Second

// wedgedVotingGrace is how long a session may legitimately sit in the voting
// status with no voting sub-document — the window inside runTelegramPollFlow
// between claiming the transition and StartVoting writing the sub-document
// (media upload + poll send). Only past this grace is such a session treated as
// wedged and cancelled, so the recovery loop never reaps a round that is
// actively launching its poll.
const wedgedVotingGrace = 2 * time.Minute

// startRecoveryLoop launches the single goroutine that drives the active
// session's lifecycle from its persisted timestamps. It is the only mechanism
// that advances deadlines, so resuming after a restart is identical to normal
// operation — the first tick simply acts on whatever the stored deadlines say.
//
// Ticks run sequentially in one goroutine (a slow tick delays the next rather
// than overlapping it), so no extra locking is needed beyond the b.mu already
// taken by the transition helpers it calls.
func (b *Bot) startRecoveryLoop() {
	go func() {
		b.recoverTick() // immediate resume on startup, before the first interval
		ticker := time.NewTicker(recoveryTickInterval)
		defer ticker.Stop()
		for range ticker.C {
			b.recoverTick()
		}
	}()
}

// recoverTick evaluates the active session once and acts on anything due.
func (b *Bot) recoverTick() {
	session, err := b.sessionRepository.GetActiveSession(context.Background())
	if err != nil {
		log.Printf("recovery: cannot get active session: %v", err)
		return
	}
	if session == nil {
		return
	}

	now := time.Now().UTC()
	switch session.Status {
	case models.StatusGathering:
		b.recoverGathering(session, now)
	case models.StatusVoting:
		b.recoverVoting(session, now)
	}
}

// recoverGathering sends the due reminder and moves to voting once the deadline
// passes (or everyone has finished/skipped).
func (b *Bot) recoverGathering(session *models.BookClubSession, now time.Time) {
	if session.Gathering.NotifiedAt == nil && !now.Before(session.Gathering.NotifyAt) {
		b.notifyGatheringDeadline(session)
		if err := b.sessionRepository.SetGatheringNotified(context.Background(), session.ID, now); err != nil {
			log.Printf("recovery: cannot mark gathering notified: %v", err)
		}
	}

	if allBooksChosen(session) || !now.Before(session.Gathering.Deadline) {
		b.runTelegramPollFlow()
	}
}

// recoverVoting sends the due reminder and closes the poll once the deadline
// passes (or everyone has voted). A voting session with no voting sub-document
// is a wedged round (the poll never started) and is cancelled to release the
// active lock.
func (b *Bot) recoverVoting(session *models.BookClubSession, now time.Time) {
	if session.Voting == nil {
		// status=voting with no voting sub-document is legitimate while
		// runTelegramPollFlow is mid-launch; only cancel once it has stayed that
		// way past the grace period (i.e. the launch crashed), so we never reap a
		// round that is actively starting its poll. UpdatedAt is stamped when the
		// status was flipped to voting.
		if now.Sub(session.UpdatedAt) > wedgedVotingGrace {
			log.Printf("recovery: voting session %s has no voting sub-document past grace, cancelling", session.ID.Hex())
			if err := b.sessionRepository.SetStatus(context.Background(), session.ID, models.StatusCancelled); err != nil {
				log.Printf("recovery: cannot cancel wedged session: %v", err)
			}
		}
		return
	}

	if session.Voting.NotifiedAt == nil && !now.Before(session.Voting.NotifyAt) {
		b.notifyPollDeadline()
		if err := b.sessionRepository.SetVotingNotified(context.Background(), session.ID, now); err != nil {
			log.Printf("recovery: cannot mark voting notified: %v", err)
		}
	}

	allVoted := session.Voting.TotalParticipants > 0 && len(session.Voting.VoterIDs) >= session.Voting.TotalParticipants
	if allVoted || !now.Before(session.Voting.Deadline) {
		b.closeTelegramPoll()
	}
}
