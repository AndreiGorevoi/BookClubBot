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

1. A subscriber runs `/start_vote`.
2. The bot DMs every active subscriber and walks each one through the book
   submission conversation, one question at a time:
   `title → author → description → cover image → done`.
   (`/skip` opts a participant out.)
3. The gathering has a **deadline**. When the deadline passes the gathering
   ends **regardless** of who has not finished — partial/absent submissions are
   simply dropped from the poll. The gathering also ends early if everyone has
   either finished or skipped.
4. A pre-deadline **reminder** is sent to participants who have not finished.

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
| `cancelled` | Aborted (e.g. fewer than 2 books gathered, bot removed) | no |

"Active" = the recovery loop is responsible for advancing it. There must be at
most **one** session whose status is active at any time (enforced by a partial
unique index — see [Indexes](#indexes)).

---

## Deadlines & recovery

Each timed phase stores **absolute timestamps**, not durations:

- `deadline` — when the phase must end.
- `notifyAt` — when to send the pre-deadline reminder.
- `notifiedAt` — set once the reminder is sent, so it fires exactly once
  (idempotency across restarts).

A single **scheduler/recovery loop** (a ticker, ~every 15s) is the only driver:

- On startup it loads the active session (if any) and resumes from its current
  status — no goroutines to re-spawn.
- On each tick, for the active session:
  - if `now >= notifyAt` and `notifiedAt` is unset → send reminder, set
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
- **Idempotent by construction.** The `notifiedAt` marker and the `status`
  transition make acting twice a no-op, so a crash mid-action is safe.
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
    "notifyAt": "2026-06-03T08:00:00Z",
    "notifiedAt": null,
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
| `notifyAt` | date | When the pre-deadline reminder is due |
| `notifiedAt` | date \| null | Set once the reminder has been sent |
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
| `step` | string | `book` \| `author` \| `description` \| `image` \| `done` \| `skipped` |
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
| `startedAt` | date | |
| `closedAt` | date \| null | `null` while the poll is open |

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
