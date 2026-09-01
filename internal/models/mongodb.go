package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Subscriber struct {
	ID        int64     `bson:"_id"`
	FirstName string    `bson:"firstName"`
	LastName  string    `bson:"lastName"`
	Nick      string    `bson:"nick"`
	Archived  bool      `bson:"archived"`
	JoinedAt  time.Time `bson:"joinedAt"`

	// Optional onboarding profile, collected by the post-subscribe question.
	// Empty means the question was skipped or never asked.
	FavoriteGenres string `bson:"favoriteGenres,omitempty"`
	// OnboardingStep is the question the subscriber is currently on. Empty means
	// not onboarding (finished, skipped, or an existing subscriber that never
	// went through it).
	//
	// NOTE: no `omitempty`. SaveSubscriber persists the whole struct via `$set`,
	// and omitempty would drop an empty value from the update — so clearing the
	// step (finishing onboarding) would never reach the DB and the subscriber
	// would be stuck onboarding forever.
	OnboardingStep string `bson:"onboardingStep"`
}

// Onboarding steps. An empty OnboardingStep means the subscriber is not
// onboarding.
const (
	OnboardingGenres = "genres"
)

// IsOnboarding reports whether the subscriber is mid-way through onboarding.
func (s *Subscriber) IsOnboarding() bool {
	return s.OnboardingStep != ""
}

// Session statuses. The first three are "active" — at most one session may be
// in any active status at a time (see SessionRepository.EnsureIndexes).
const (
	StatusGathering = "gathering"
	StatusVoting    = "voting"
	StatusReading   = "reading"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
)

// Participant submission steps during book gathering.
const (
	StepBook        = "book"
	StepAuthor      = "author"
	StepDescription = "description"
	StepImage       = "image"
	StepReview      = "review"
	StepDone        = "done"
	StepSkipped     = "skipped"
)

// Reading member statuses (step 3, reserved for future use).
const (
	ReadingInProgress = "reading"
	ReadingFinished   = "finished"
	ReadingAbandoned  = "abandoned"
)

// Book is a single book submission (partial while a participant is still
// answering questions, complete once their step reaches StepDone).
type Book struct {
	Title       string `bson:"title"`
	Author      string `bson:"author"`
	Description string `bson:"description"`
	PhotoID     string `bson:"photoId"`
}

// Participant holds one subscriber's in-progress conversation state during book
// gathering, so a restart resumes them exactly where they left off.
type Participant struct {
	SubscriberID int64      `bson:"subscriberId"`
	FirstName    string     `bson:"firstName"`
	LastName     string     `bson:"lastName"`
	Nick         string     `bson:"nick"`
	Step         string     `bson:"step"`
	Book         *Book      `bson:"book"`
	InvitedAt    time.Time  `bson:"invitedAt"`
	SubmittedAt  *time.Time `bson:"submittedAt"`
}

// Gathering is the book-collection phase (step 1).
type Gathering struct {
	Deadline     time.Time      `bson:"deadline"`
	NotifyAt     time.Time      `bson:"notifyAt"`
	NotifiedAt   *time.Time     `bson:"notifiedAt"`
	Participants []*Participant `bson:"participants"`
}

// Voting is the Telegram poll phase (step 2).
type Voting struct {
	TelegramPollID    int        `bson:"telegramPollId"`
	Deadline          time.Time  `bson:"deadline"`
	NotifyAt          time.Time  `bson:"notifyAt"`
	NotifiedAt        *time.Time `bson:"notifiedAt"`
	TotalParticipants int        `bson:"totalParticipants"`
	VoterIDs          []int64    `bson:"voterIds"`
	// OptionOwners holds the subscriber id behind each poll option, in the same
	// order the options were sent to Telegram. Winners are resolved by position
	// through this list, so two books whose options render to the same text stay
	// distinct. Empty for polls started before this field existed.
	OptionOwners []int64    `bson:"optionOwners"`
	StartedAt    time.Time  `bson:"startedAt"`
	ClosedAt     *time.Time `bson:"closedAt"`
}

// Winner is a winning book. A round can have several winners on a tie.
type Winner struct {
	SubscriberID int64  `bson:"subscriberId"`
	Title        string `bson:"title"`
	Author       string `bson:"author"`
}

// ReadingMember is one subscriber's progress and review of the winning book
// (step 3, reserved for future use).
type ReadingMember struct {
	SubscriberID int64      `bson:"subscriberId"`
	Status       string     `bson:"status"`
	Rating       *int       `bson:"rating"`
	Review       *string    `bson:"review"`
	StartedAt    time.Time  `bson:"startedAt"`
	FinishedAt   *time.Time `bson:"finishedAt"`
}

// Reading is the post-vote reading phase (step 3, reserved for future use).
type Reading struct {
	Book    Book             `bson:"book"`
	Members []*ReadingMember `bson:"members"`
}

// BookClubSession is one complete round: gathering → voting → reading.
type BookClubSession struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	Name      string             `bson:"name"`
	Status    string             `bson:"status"`
	CreatedBy int64              `bson:"createdBy"`
	CreatedAt time.Time          `bson:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt"`
	Gathering Gathering          `bson:"gathering"`
	Voting    *Voting            `bson:"voting"`
	Winners   []Winner           `bson:"winners"`
	Reading   *Reading           `bson:"reading"`

	// ActiveLock is present only while the session is in an active status. A
	// unique partial index on its existence guarantees at most one active
	// session at a time. It is never read by application code; SessionRepository
	// sets and unsets it as the status changes.
	ActiveLock *bool `bson:"activeLock,omitempty"`
}

// IsActiveStatus reports whether a status is active (non-terminal). It is the
// single source of truth for which statuses count as active.
func IsActiveStatus(status string) bool {
	switch status {
	case StatusGathering, StatusVoting, StatusReading:
		return true
	default:
		return false
	}
}

// IsActive reports whether a session is in an active (non-terminal) status.
func (s *BookClubSession) IsActive() bool {
	return IsActiveStatus(s.Status)
}
