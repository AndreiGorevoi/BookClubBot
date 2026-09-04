# Book Club Flow & Session Schema

This document describes the **business flow** of a book club round and the
**MongoDB schema** that backs it. It is the source of truth for the
session feature — it supersedes the "Models defined but not yet persisted"
section of [`db-schema.md`](./db-schema.md).

The goal of the feature is **persistence**: an in-flight gathering or poll must
survive a process restart. Today the round lives only in memory
(`bot.bookGathering` + `bot.telegramPoll`) and the lifecycle is driven by
`time.Sleep` goroutines, so a restart loses everything. Moving the state to
MongoDB and driving deadlines from persisted timestamps fixes both.

---

## The flow

A **session** is one complete round. There is **at most one active session at a
time** (a second `/start_vote` while one is live is rejected).

### Step 1 — Book gathering

1. An admin runs `/start_vote` (admins are the Telegram user ids listed in
   `admin_ids` / the `ADMIN_IDS` env var; anyone else is refused).
2. The bot DMs every active subscriber and walks each one through the book
   submission conversation, one question at a time:
   `title → author → description → cover image → review → done`.
   At the **review** step the bot shows a summary of everything entered with two
   inline buttons: confirm (submit the book, → `done`) or start over (discard the
   book and go back to `title`). (`/skip` opts a participant out.)
3. The gathering has a **deadline**. When the deadline passes the gathering
   ends **regardless** of who has not finished — partial/absent submissions are
   simply dropped from the poll. The gathering also ends early if everyone has
   either finished or skipped.
4. **Reminders** are anchored backwards from the deadline: one is due at
   `deadline - k*interval` for every `k >= 1` whose point still falls strictly
   inside the gathering window, where the interval is the
   `gathering_reminder_interval` config key (`0` disables reminders entirely).
   Every reminder is a **group message mentioning the participants who have not
   submitted** — members who skipped are never mentioned. The `k == 1` reminder
   is the **last call** and additionally DMs those participants.

   Anchoring backwards rather than forwards from the start is what puts the
   *final* reminder at a known distance from the deadline; counting forwards
   would leave it at an arbitrary offset depending on how the interval divides
   the window. The group message is sent with **HTML** parse mode (unlike the
   Markdown used elsewhere) because it embeds member-supplied names, and a
   Telegram username may legitimately contain `_`.

5. **Quiet hours** (`quiet_hours_start` / `quiet_hours_end` / `timezone`,
   default 23:00–08:00 Europe/Warsaw) suppress reminders overnight. A reminder
   that comes due inside the window is **held, not dropped**: `remindersSent` is
   left untouched, so the first tick after the window closes still sees it as
   owed and sends it then — and a whole night's worth collapses into the single
   message the backlog rule already produces. Dropping instead would risk
   silently swallowing the last call, the only reminder that also DMs. An empty
   `timezone`, or equal bounds, disables quiet hours.

   Holding only applies while the window closes **before the deadline**. When it
   would outlast the deadline, the reminder is sent inside the quiet window
   instead, because waiting would drop it rather than delay it: the gathering
   would end before the window ever closed. With prod's shape (24h window, 6h
   interval, 23:00–08:00) this is what rounds started between 05:00 and 08:00
   depend on — their last call falls at 23:00–02:00 with a deadline before the
   window closes. A message at an antisocial hour beats no last call at all.

   The runtime image is alpine without the `tzdata` package, so `cmd/main.go`
   imports `_ "time/tzdata"` to embed the timezone database in the binary;
   without it `time.LoadLocation` fails in the container.

### Step 2 — Voting

1. The bot posts the collected books as a native Telegram poll in the group.
2. The poll has a **deadline**. It closes when the deadline passes **or** when
   every eligible subscriber has voted, whichever comes first.
3. A pre-deadline **reminder** is sent to the group.
4. On close the bot tallies votes and announces the winner. Ties are possible
   (`winners` can hold more than one book) and fall back to manual resolution.

### Step 3 — Reading & reviews (future, not implemented yet)

After a winner is chosen, the club reads **the winning book**. Every subscriber:

1. Gets a reading status (`reading → finished`).
2. Returns to the bot when done to submit a **rating** and a short **review**.

The schema below already reserves a `reading` sub-document for this so step 3
can be added without a migration. Do not build the step-3 behaviour yet — just
keep the shape stable.

---

## Lifecycle / status

A session moves through these statuses:

| Status | Meaning | Active? |
|---|---|---|
| `gathering` | Collecting book submissions (step 1) | yes |
| `voting` | Telegram poll is open (step 2) | yes |
| `reading` | Winner chosen, club is reading (step 3, future) | yes |
| `completed` | Round finished and archived | no |
| `cancelled` | Aborted (fewer than 2 books gathered, bot removed, or ended early from the admin console) | no |

"Active" = the recovery loop is responsible for advancing it. There must be at
most **one** session whose status is active at any time (enforced by a partial
unique index — see [Indexes](#indexes)).

An admin can end an active round early from the `/admin` console (see
[Admin console](#admin-console)); it always lands on `cancelled`, whatever
phase the round was in.

---

## Deadlines & recovery

Each timed phase stores an **absolute deadline**, not a duration, plus a marker
that makes its reminder idempotent across restarts:

- `deadline` — when the phase must end.
- gathering: `remindersSent` — how many of the reminders scheduled backwards
  from `deadline` have been sent. The schedule itself is derived from the
  deadline and the configured interval, so nothing else needs storing.
- voting: `notifyAt` / `notifiedAt` — when the single pre-deadline reminder is
  due, and when it was sent.

A single **scheduler/recovery loop** (a ticker, ~every 15s) is the only driver:

- On startup it loads the active session (if any) and resumes from its current
  status — no goroutines to re-spawn.
- On each tick, for the active session:
  - gathering: count the schedule points that are now due; if that count exceeds
    `remindersSent` → send **one** reminder (the most recent due point) and store
    the new count. Because the count is monotonic in `now`, a backlog built up
    during downtime collapses into a single message instead of a burst;
  - voting: if `now >= notifyAt` and `notifiedAt` is unset → send reminder, set
    `notifiedAt`;
  - if `now >= deadline` → end the phase (gathering → start poll, or
    voting → close poll & announce);
  - for voting only: if all eligible subscribers have voted
    (`len(voterIds) >= totalParticipants`) → close early.

Because every action is keyed off persisted state and guarded by an idempotency
marker, a crash at any point is safe — the next tick re-evaluates and continues.

### Why a ticker loop (and not per-deadline timers)

The driver is **one ticker goroutine**, started once in `Run()`:

```go
ticker := time.NewTicker(15 * time.Second)
for range ticker.C {
    b.checkActiveSession(ctx) // load the active session, act on due deadlines
}
```

This is deliberately chosen over per-deadline `time.AfterFunc`/`time.Sleep`
goroutines (the current in-memory approach):

- **Recovery is free.** The loop doesn't care when it started. The first tick
  after a restart simply sees `now >= deadline` and fires — there are no
  remaining durations to recompute and no goroutines to re-spawn. Catching up
  after downtime and normal operation are the *same* code path, so there is no
  separate recovery path to get wrong.
- **Idempotent by construction.** The `remindersSent` / `notifiedAt` markers and
  the `status` transition make acting twice a no-op, so a crash mid-action is
  safe.
- **One code path** handles both "deadline passed while running" and "deadline
  passed while we were down."

Trade-off: a deadline may fire up to one tick (~15s) late. For a book club
measured in days this is irrelevant. Rejected alternatives: per-deadline timers
(restart-fragile), MongoDB TTL indexes (only delete documents, can't run
logic), and change streams / external cron (extra infra a single-instance bot
doesn't need).

Implementation notes:

- **No overlapping ticks.** Run the per-tick checks sequentially in the single
  ticker goroutine (or guard with a "busy" flag) so a long-running tick can't
  start a second concurrent pass over the same session.
- **Poll early-close has two paths.** The "all eligible voted"
  (`len(voterIds) >= totalParticipants`) check runs inside the tick as the
  safety net, while a live `PollAnswer` update can still close the poll
  immediately as the fast path. Both converge on the same idempotent close
  routine.

---

## Admin console

`/admin` opens an inline-button panel in the caller's DM, restricted to the
configured admins (`bot.isAdmin`, see [Configuration](../CLAUDE.md)). It is a
single message edited in place: every button re-renders the same message rather
than posting a new one.

| View | Does |
|---|---|
| Members | Lists the active subscribers, paginated |
| Round | Phase, deadline, who has submitted / is pending / skipped, votes cast |
| End round | Cancels the active round after a confirmation, and tells the group |
| Unsubscribe | Archives a chosen member after a confirmation |

Two rules the console lives by:

- **Authorization is re-checked on every press.** Inline buttons stay in chat
  history forever, so a panel opened while someone was an admin must not keep
  working after they are removed from the config.
- **Destructive actions take two presses.** Ending a round and unsubscribing a
  member each render a confirmation view first; nothing is written on the way
  there.

Ending a round writes the status under `bot.mu`, the same lock every other phase
transition takes, so it cannot race the recovery loop. Two ordering rules make
that safe:

- **The poll is closed before the status becomes terminal**, the same way
  `closeTelegramPoll` completes a round only after its own `StopPoll` succeeds.
  Once a session is cancelled it is no longer active, so nothing would ever
  retry the stop; a failed stop therefore leaves the round alone and the admin
  can press again.
- **`StartVoting` only writes to a session that still holds the active lock.**
  A poll is sent to Telegram before the voting sub-document is persisted, so a
  round can be ended in that window; without the guard the late write would put
  `status` and `activeLock` back and resurrect it. The refused write comes back
  as `ErrNotFound`, and the poll that was already sent is closed.

Unsubscribing a member also releases the round in flight: a member still owing a
book is moved to `skipped`, so the reminders stop mentioning them and
`allBooksChosen` is no longer waiting on them. A member who already submitted
keeps their book on the ballot.

---

## Schema

MongoDB database: **`book_club_boot`**. New collection: **`book_club_sessions`**.

```json
{
  "_id": "<ObjectID>",
  "name": "June 2026",
  "status": "gathering",
  "createdBy": 123456789,
  "createdAt": "2026-06-01T10:00:00Z",
  "updatedAt": "2026-06-01T10:00:00Z",

  "gathering": {
    "deadline": "2026-06-03T10:00:00Z",
    "remindersSent": 0,
    "participants": [
      {
        "subscriberId": 123456789,
        "firstName": "Andrei",
        "lastName": "Haravy",
        "nick": "andreiharavy",
        "step": "author",
        "book": {
          "title": "The Pragmatic Programmer",
          "author": "",
          "description": "",
          "photoId": ""
        },
        "invitedAt": "2026-06-01T10:00:00Z",
        "submittedAt": null
      }
    ]
  },

  "voting": {
    "telegramPollId": 42,
    "deadline": "2026-06-04T10:00:00Z",
    "notifyAt": "2026-06-04T08:00:00Z",
    "notifiedAt": null,
    "totalParticipants": 5,
    "voterIds": [123456789, 987654321],
    "optionOwners": [987654321, 123456789],
    "startedAt": "2026-06-03T10:00:00Z",
    "closedAt": null
  },

  "winners": [
    {
      "subscriberId": 123456789,
      "title": "The Pragmatic Programmer",
      "author": "David Thomas"
    }
  ],

  "reading": null
}
```

### `book_club_sessions` — top level

| Field | BSON type | Notes |
|---|---|---|
| `_id` | ObjectID | Auto-generated |
| `name` | string | Human label, auto-generated (e.g. `"June 2026"`) |
| `status` | string | One of the lifecycle statuses above |
| `createdBy` | int64 | Telegram user ID who ran `/start_vote` |
| `createdAt` | date | Session creation time |
| `updatedAt` | date | Last mutation; bumped on every write |
| `gathering` | object | Step 1 sub-document |
| `voting` | object \| null | Step 2 sub-document; `null` until the poll starts |
| `winners` | array | 0 (no winner / cancelled), 1, or many (tie) entries |
| `reading` | object \| null | Step 3 sub-document; `null` until reserved for future use |
| `activeLock` | bool (present only while active) | Internal lock backing the unique "one active session" index; omitted in terminal states. See [Indexes](#indexes) |

### `gathering`

| Field | BSON type | Notes |
|---|---|---|
| `deadline` | date | When gathering force-ends |
| `remindersSent` | int32 | How many reminders scheduled backwards from `deadline` have been sent; the next one fires only once a further point comes due |
| `participants` | array | Embedded `Participant` objects |

### `Participant` (embedded)

Holds **in-progress conversation state** so a restart resumes each user exactly
where they left off. A finished participant's `book` is the completed
submission; the poll is built from participants whose `step` is `done`.

| Field | BSON type | Notes |
|---|---|---|
| `subscriberId` | int64 | References `subscribers._id` |
| `firstName` | string | Snapshot at invite time |
| `lastName` | string | Snapshot |
| `nick` | string | Snapshot |
| `step` | string | `book` \| `author` \| `description` \| `image` \| `review` \| `done` \| `skipped` |
| `book` | object \| null | Partial while in progress, complete when `step == done` |
| `invitedAt` | date | When the bot DMed this participant |
| `submittedAt` | date \| null | When `step` reached `done` |

**`book` (embedded):**

| Field | BSON type | Notes |
|---|---|---|
| `title` | string | |
| `author` | string | |
| `description` | string | |
| `photoId` | string | Telegram `FileID`; empty string if no cover submitted |

### `voting`

| Field | BSON type | Notes |
|---|---|---|
| `telegramPollId` | int32 | Telegram **message ID** of the poll |
| `deadline` | date | When the poll force-closes |
| `notifyAt` | date | When the pre-deadline reminder is due |
| `notifiedAt` | date \| null | Set once the reminder has been sent |
| `totalParticipants` | int32 | Snapshot of eligible voter count at poll start |
| `voterIds` | array<int64> | Unique voters; powers dedup, count, and early close |
| `optionOwners` | array<int64> | Subscriber behind each poll option, in ballot order; resolves winners by position |
| `startedAt` | date | |
| `closedAt` | date \| null | `null` while the poll is open |

> `optionOwners` is what maps a winning option back to a book. Matching on the
> rendered option text cannot: options are capped at Telegram's 100-unit limit,
> so two books that differ only past the cap render identically. The array is
> written in the same (shuffled, ten-option) order the options were sent, and it
> must stay in step with them — see `extractPollOptions` and `runTelegramPoll`.
> A poll started before this field existed has it empty, and `winnersFromPoll`
> falls back to text matching for those.

> `voterIds` replaces the old `participantsVoted` counter. A bare count cannot
> survive a restart without risking double-counting, since Telegram does not
> reliably re-deliver historical `PollAnswer` updates.

### `Winner` (embedded array element)

| Field | BSON type | Notes |
|---|---|---|
| `subscriberId` | int64 | Who suggested the winning book |
| `title` | string | Copied from the winning submission |
| `author` | string | |

### `reading` (future — step 3)

`null` for now. Reserved shape:

```json
{
  "book": {
    "title": "The Pragmatic Programmer",
    "author": "David Thomas",
    "photoId": "AgACAgIAAxk...",
    "subscriberId": 123456789
  },
  "members": [
    {
      "subscriberId": 123456789,
      "status": "reading",
      "rating": null,
      "review": null,
      "startedAt": "2026-06-05T10:00:00Z",
      "finishedAt": null
    }
  ]
}
```

| Field | BSON type | Notes |
|---|---|---|
| `book` | object | The winning book the club reads |
| `members[].subscriberId` | int64 | References `subscribers._id` |
| `members[].status` | string | `reading` \| `finished` \| `abandoned` |
| `members[].rating` | int32 \| null | e.g. 1–5; `null` until submitted |
| `members[].review` | string \| null | Free text; `null` until submitted |
| `members[].startedAt` | date | When reading began |
| `members[].finishedAt` | date \| null | When the member finished |

---

## Indexes

| Index | Purpose |
|---|---|
| Unique partial on `activeLock` where `activeLock` exists | Enforce **one active session at a time** |
| `createdAt: -1` | List past sessions / fetch the latest for history |

### Why `activeLock` instead of an index on `status`

Conceptually we want "at most one session whose `status ∈ {gathering, voting,
reading}`". The natural index would be a unique partial index with
`partialFilterExpression: { status: { $in: [...] } }` — but **`$in` inside a
partial index filter is only supported from MongoDB 6.3**, and CI/prod run
**MongoDB 6.0**. `$exists` is supported on all versions, so instead each session
carries an internal `activeLock` field that is **present only while active** and
absent once it reaches a terminal status. A unique index over the docs where
`activeLock` exists then permits exactly one active session.

`activeLock` is managed entirely by `SessionRepository` (set on create / active
transitions, unset on `completed`/`cancelled`) and is never read by application
logic — `status` remains the source of truth for the phase.

---

## History

History falls out of the schema for free: completed rounds stay in
`book_club_sessions` with `status: completed` and `winners` populated.

- "What are we reading this month" → latest session with status `reading` /
  `completed`, look at `winners` (or `reading.book`).
- "What has been suggested" → `gathering.participants[].book` across sessions.

Planned read helpers: `GetActiveSession`, `GetCurrentBook`, `ListPastSessions`.
