package bot

import (
	"BookClubBot/config"
	"BookClubBot/internal/models"
	"BookClubBot/message"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// gatheringAt builds a session whose gathering started at start and runs for
// window, with the given participant steps.
func gatheringAt(start time.Time, window time.Duration, steps ...string) *models.BookClubSession {
	participants := make([]*models.Participant, 0, len(steps))
	for i, step := range steps {
		participants = append(participants, &models.Participant{
			SubscriberID: int64(i + 1),
			FirstName:    "Member",
			Step:         step,
		})
	}
	return &models.BookClubSession{
		ID:        primitive.NewObjectID(),
		Status:    models.StatusGathering,
		CreatedAt: start,
		Gathering: models.Gathering{
			Deadline:     start.Add(window),
			Participants: participants,
		},
	}
}

func TestDueReminderCount(t *testing.T) {
	start := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)

	data := map[string]struct {
		window   time.Duration
		interval time.Duration
		elapsed  time.Duration // how far past start "now" is
		expected int
	}{
		// A 24h window with a 6h interval schedules points at 6h, 12h and 18h
		// past the start (deadline-18h, -12h, -6h). deadline-24h is the start
		// itself and is excluded.
		`nothing due at the start`:            {24 * time.Hour, 6 * time.Hour, 0, 0},
		`nothing due just before first point`: {24 * time.Hour, 6 * time.Hour, 6*time.Hour - time.Second, 0},
		`first point due`:                     {24 * time.Hour, 6 * time.Hour, 6 * time.Hour, 1},
		`second point due`:                    {24 * time.Hour, 6 * time.Hour, 12 * time.Hour, 2},
		`last point due`:                      {24 * time.Hour, 6 * time.Hour, 18 * time.Hour, 3},
		`between points does not advance`:     {24 * time.Hour, 6 * time.Hour, 15 * time.Hour, 2},

		// The count is capped by the schedule, so it stays stable once the
		// deadline is reached or passed.
		`at the deadline`:     {24 * time.Hour, 6 * time.Hour, 24 * time.Hour, 3},
		`past the deadline`:   {24 * time.Hour, 6 * time.Hour, 48 * time.Hour, 3},
		`long after downtime`: {24 * time.Hour, 6 * time.Hour, 30 * 24 * time.Hour, 3},

		// An interval that does not divide the window still puts the final point
		// exactly one interval before the deadline — the whole reason for
		// anchoring backwards.
		`indivisible interval, first point`: {24 * time.Hour, 7 * time.Hour, 3 * time.Hour, 1},
		`indivisible interval, all points`:  {24 * time.Hour, 7 * time.Hour, 17 * time.Hour, 3},

		// An interval at least as long as the window leaves no point strictly
		// inside it: the only candidate would fall on the start itself, when
		// members have just been DMed the prompt.
		`interval equals window`:  {24 * time.Hour, 24 * time.Hour, 24 * time.Hour, 0},
		`interval exceeds window`: {1 * time.Hour, 2 * time.Hour, 1 * time.Hour, 0},

		// Prod today: a 24h gathering with a 12h interval yields the single
		// reminder 12h before the deadline that notify_before_gathering used to
		// send.
		`prod shape, before the point`: {24 * time.Hour, 12 * time.Hour, 11 * time.Hour, 0},
		`prod shape, at the point`:     {24 * time.Hour, 12 * time.Hour, 12 * time.Hour, 1},
		`prod shape, at the deadline`:  {24 * time.Hour, 12 * time.Hour, 24 * time.Hour, 1},

		`disabled by zero interval`:     {24 * time.Hour, 0, 12 * time.Hour, 0},
		`disabled by negative interval`: {24 * time.Hour, -time.Hour, 12 * time.Hour, 0},
		`non-positive window`:           {0, 6 * time.Hour, time.Hour, 0},
	}

	for name, tt := range data {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := dueReminderCount(start, start.Add(tt.window), tt.interval, start.Add(tt.elapsed))
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestDueReminderCountIsMonotonic(t *testing.T) {
	start := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	deadline := start.Add(24 * time.Hour)
	interval := 5 * time.Hour

	// The count doubles as the persisted "how many have been handled" marker, so
	// it must never decrease as time moves forward.
	prev := 0
	for elapsed := time.Duration(0); elapsed <= 30*time.Hour; elapsed += 7 * time.Minute {
		got := dueReminderCount(start, deadline, interval, start.Add(elapsed))
		assert.GreaterOrEqual(t, got, prev, "count went backwards at elapsed=%s", elapsed)
		prev = got
	}
}

func TestPendingParticipants(t *testing.T) {
	start := time.Now().UTC()
	session := gatheringAt(start, time.Hour,
		models.StepBook,
		models.StepAuthor,
		models.StepDescription,
		models.StepImage,
		models.StepReview,
		models.StepDone,
		models.StepSkipped,
	)

	pending := pendingParticipants(session)

	// Everyone who has not finished is still pending, including members who
	// started the flow but stalled mid-way. Members who opted out are never
	// nudged.
	steps := make([]string, 0, len(pending))
	for _, p := range pending {
		steps = append(steps, p.Step)
	}
	assert.Equal(t, []string{
		models.StepBook,
		models.StepAuthor,
		models.StepDescription,
		models.StepImage,
		models.StepReview,
	}, steps)
}

func TestPendingParticipantsAllSettled(t *testing.T) {
	session := gatheringAt(time.Now().UTC(), time.Hour, models.StepDone, models.StepSkipped)
	assert.Empty(t, pendingParticipants(session))
}

func TestPlanGatheringReminder(t *testing.T) {
	start := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	const window = 24 * time.Hour
	const interval = 6 * time.Hour

	t.Run("nothing due yet", func(t *testing.T) {
		session := gatheringAt(start, window, models.StepBook)
		plan := planGatheringReminder(session, interval, quietHours{}, start.Add(time.Hour))
		assert.False(t, plan.send)
	})

	t.Run("first point due sends a group-only reminder", func(t *testing.T) {
		session := gatheringAt(start, window, models.StepBook, models.StepDone)
		plan := planGatheringReminder(session, interval, quietHours{}, start.Add(6*time.Hour))

		assert.True(t, plan.send)
		assert.Equal(t, 1, plan.dueCount)
		assert.False(t, plan.lastCall, "18h before the deadline is not the last call")
		assert.Equal(t, 18*time.Hour, plan.remaining)
		assert.Len(t, plan.recipients, 1)
	})

	t.Run("already sent within the same interval", func(t *testing.T) {
		session := gatheringAt(start, window, models.StepBook)
		session.Gathering.RemindersSent = 2

		plan := planGatheringReminder(session, interval, quietHours{}, start.Add(12*time.Hour))
		assert.False(t, plan.send, "the point at 12h is already accounted for")

		// Still nothing at the very end of that interval...
		plan = planGatheringReminder(session, interval, quietHours{}, start.Add(18*time.Hour-time.Second))
		assert.False(t, plan.send)

		// ...until the next point comes due.
		plan = planGatheringReminder(session, interval, quietHours{}, start.Add(18*time.Hour))
		assert.True(t, plan.send)
		assert.Equal(t, 3, plan.dueCount)
	})

	t.Run("backlog after downtime collapses into one reminder", func(t *testing.T) {
		session := gatheringAt(start, window, models.StepBook)

		// The bot was down across the 6h and 12h points and comes back at 12h.
		plan := planGatheringReminder(session, interval, quietHours{}, start.Add(12*time.Hour))

		assert.True(t, plan.send)
		assert.Equal(t, 2, plan.dueCount, "both missed points are marked handled at once")
		assert.False(t, plan.lastCall)
	})

	t.Run("last call is the k==1 reminder", func(t *testing.T) {
		session := gatheringAt(start, window, models.StepBook)
		session.Gathering.RemindersSent = 2

		plan := planGatheringReminder(session, interval, quietHours{}, start.Add(18*time.Hour))

		assert.True(t, plan.send)
		assert.True(t, plan.lastCall, "one interval before the deadline is the last call")
		assert.Equal(t, interval, plan.remaining)
	})

	t.Run("no reminder once the deadline has passed", func(t *testing.T) {
		// The same tick ends the gathering and starts the poll, so a reminder here
		// would arrive seconds before the round moves on. This is what a restart
		// after downtime spanning the whole window would otherwise do.
		session := gatheringAt(start, window, models.StepBook)

		assert.False(t, planGatheringReminder(session, interval, quietHours{}, start.Add(window)).send)
		assert.False(t, planGatheringReminder(session, interval, quietHours{}, start.Add(window+72*time.Hour)).send)
	})

	t.Run("last call still fires just before the deadline", func(t *testing.T) {
		session := gatheringAt(start, window, models.StepBook)

		plan := planGatheringReminder(session, interval, quietHours{}, start.Add(window-time.Second))

		assert.True(t, plan.send)
		assert.True(t, plan.lastCall)
		assert.Equal(t, time.Second, plan.remaining)
	})

	t.Run("nobody to nudge", func(t *testing.T) {
		session := gatheringAt(start, window, models.StepDone, models.StepSkipped)
		plan := planGatheringReminder(session, interval, quietHours{}, start.Add(18*time.Hour))
		assert.False(t, plan.send)
	})

	t.Run("zero interval disables reminders entirely", func(t *testing.T) {
		session := gatheringAt(start, window, models.StepBook)
		plan := planGatheringReminder(session, 0, quietHours{}, start.Add(window))
		assert.False(t, plan.send, "not even the last call fires when disabled")
	})
}

func TestPlanGatheringReminderProdShape(t *testing.T) {
	// A 24h gathering with a 12h interval must behave exactly as the removed
	// notify_before_gathering did: a single reminder 12h before the deadline,
	// which is also the last call and therefore DMs.
	start := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	session := gatheringAt(start, 24*time.Hour, models.StepBook)
	interval := 12 * time.Hour

	assert.False(t, planGatheringReminder(session, interval, quietHours{}, start.Add(11*time.Hour)).send)

	plan := planGatheringReminder(session, interval, quietHours{}, start.Add(12*time.Hour))
	assert.True(t, plan.send)
	assert.Equal(t, 1, plan.dueCount)
	assert.True(t, plan.lastCall)

	// And it does not repeat for the rest of the window.
	session.Gathering.RemindersSent = plan.dueCount
	assert.False(t, planGatheringReminder(session, interval, quietHours{}, start.Add(23*time.Hour)).send)
}

func TestMention(t *testing.T) {
	data := map[string]struct {
		participant *models.Participant
		expected    string
	}{
		`username is mentioned by name`: {
			&models.Participant{SubscriberID: 1, Nick: "vasya", FirstName: "Vasya"},
			"@vasya",
		},
		`underscores in a username are left intact`: {
			// Telegram usernames may contain '_', which is why this message is sent
			// as HTML rather than Markdown.
			&models.Participant{SubscriberID: 1, Nick: "vasya_p"},
			"@vasya_p",
		},
		`no username falls back to an inline link`: {
			&models.Participant{SubscriberID: 456, FirstName: "Мария"},
			`<a href="tg://user?id=456">Мария</a>`,
		},
		`full name is used when available`: {
			&models.Participant{SubscriberID: 7, FirstName: "Ada", LastName: "Lovelace"},
			`<a href="tg://user?id=7">Ada Lovelace</a>`,
		},
		`html special characters in a name are escaped`: {
			&models.Participant{SubscriberID: 9, FirstName: "<b>Bob</b>", LastName: "& co"},
			`<a href="tg://user?id=9">&lt;b&gt;Bob&lt;/b&gt; &amp; co</a>`,
		},
		`nameless member falls back to their id`: {
			&models.Participant{SubscriberID: 42},
			`<a href="tg://user?id=42">42</a>`,
		},
		`last name only`: {
			&models.Participant{SubscriberID: 3, LastName: "Turing"},
			`<a href="tg://user?id=3">Turing</a>`,
		},
	}

	for name, tt := range data {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, mention(tt.participant))
		})
	}
}

func TestMentionList(t *testing.T) {
	got := mentionList([]*models.Participant{
		{SubscriberID: 1, Nick: "vasya"},
		{SubscriberID: 456, FirstName: "Мария"},
		{SubscriberID: 2, Nick: "petya"},
	})

	assert.Equal(t, `@vasya, <a href="tg://user?id=456">Мария</a>, @petya`, got)
}

func TestMentionListEmpty(t *testing.T) {
	assert.Equal(t, "", mentionList(nil))
}

func TestRemindAboutGatheringPersistsCounter(t *testing.T) {
	start := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	session := gatheringAt(start, 24*time.Hour, models.StepBook)

	newBot := func(fake *fakeSessionRepo) *Bot {
		return &Bot{
			// GroupId 0 keeps the send a no-op, so the test exercises the wiring
			// from plan to persisted counter without a Telegram client.
			cfg:               &config.AppConfig{GatheringReminderInterval: int((6 * time.Hour).Seconds())},
			messages:          &message.LocalizedMessages{GatheringReminder: "%.f %s"},
			sessionRepository: fake,
		}
	}

	t.Run("due reminder is recorded", func(t *testing.T) {
		fake := &fakeSessionRepo{}
		newBot(fake).remindAboutGathering(session, start.Add(12*time.Hour))

		assert.Equal(t, 1, fake.gatherNotify)
		assert.Equal(t, 2, fake.remindersSent, "both missed points are marked handled")
	})

	t.Run("nothing due writes nothing", func(t *testing.T) {
		fake := &fakeSessionRepo{}
		newBot(fake).remindAboutGathering(session, start.Add(time.Hour))

		assert.Zero(t, fake.gatherNotify)
	})
}

func TestQuietHoursCovers(t *testing.T) {
	// A fixed offset stands in for a real zone: covers() only looks at the local
	// hour, so this exercises the same logic without depending on the host's
	// timezone database.
	warsaw := time.FixedZone("CEST", 2*60*60)
	night := quietHours{start: 23, end: 8, loc: warsaw}

	data := map[string]struct {
		hours   quietHours
		utcHour int
		covered bool
	}{
		// 23:00-08:00 local, i.e. 21:00-06:00 UTC at +2.
		`just before the window`: {night, 20, false},
		`window opens`:           {night, 21, true},
		`around midnight`:        {night, 23, true},
		`after midnight`:         {night, 2, true},
		`last quiet hour`:        {night, 5, true},
		`window closes`:          {night, 6, false},
		`midday`:                 {night, 10, false},

		// A window that does not wrap midnight.
		`daytime window, inside`:  {quietHours{start: 10, end: 14, loc: warsaw}, 9, true},
		`daytime window, outside`: {quietHours{start: 10, end: 14, loc: warsaw}, 13, false},

		// Disabled forms.
		`no timezone disables the window`: {quietHours{start: 23, end: 8}, 2, false},
		`equal bounds disable the window`: {quietHours{start: 8, end: 8, loc: warsaw}, 2, false},
	}

	for name, tt := range data {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			at := time.Date(2026, 6, 1, tt.utcHour, 30, 0, 0, time.UTC)
			assert.Equal(t, tt.covered, tt.hours.covers(at))
		})
	}
}

func TestQuietHoursHoldRatherThanDropReminders(t *testing.T) {
	// Gathering runs 09:00 → 09:00 local with a 6h interval, so points fall at
	// 15:00, 21:00 and 03:00 local. The 03:00 point is the last call and lands
	// inside quiet hours.
	warsaw := time.FixedZone("CEST", 2*60*60)
	night := quietHours{start: 23, end: 8, loc: warsaw}
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, warsaw)
	session := gatheringAt(start, 24*time.Hour, models.StepBook)
	const interval = 6 * time.Hour

	local := func(day, hour int) time.Time {
		return time.Date(2026, 6, day, hour, 0, 0, 0, warsaw)
	}

	// The two daytime points go out normally.
	plan := planGatheringReminder(session, interval, night, local(1, 15))
	assert.True(t, plan.send)
	session.Gathering.RemindersSent = plan.dueCount

	plan = planGatheringReminder(session, interval, night, local(1, 21))
	assert.True(t, plan.send)
	session.Gathering.RemindersSent = plan.dueCount

	// The 03:00 point is due but falls in the quiet window, so nothing is sent
	// and — crucially — the counter is left alone.
	held := planGatheringReminder(session, interval, night, local(2, 3))
	assert.False(t, held.send, "a reminder due at 03:00 must not be sent")
	assert.Equal(t, 2, session.Gathering.RemindersSent, "the held point must not be marked handled")

	// Still held right up to the end of the window...
	assert.False(t, planGatheringReminder(session, interval, night, local(2, 7)).send)

	// ...and delivered as soon as it closes, still flagged as the last call so it
	// also DMs. Dropping it would have silenced the only DM of the round.
	resumed := planGatheringReminder(session, interval, night, local(2, 8))
	assert.True(t, resumed.send)
	assert.Equal(t, 3, resumed.dueCount)
	assert.True(t, resumed.lastCall)
}

func TestQuietHoursDisabledSendsAtNight(t *testing.T) {
	warsaw := time.FixedZone("CEST", 2*60*60)
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, warsaw)
	session := gatheringAt(start, 24*time.Hour, models.StepBook)
	session.Gathering.RemindersSent = 2

	at := time.Date(2026, 6, 2, 3, 0, 0, 0, warsaw)
	assert.True(t, planGatheringReminder(session, 6*time.Hour, quietHours{}, at).send)
}
