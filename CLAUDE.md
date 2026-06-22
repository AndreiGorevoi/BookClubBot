# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run the bot
go run ./cmd/main.go

# Build
go build -o book-club-bot ./cmd/main.go

# Run all tests (requires MongoDB at localhost:27017)
go test ./...

# Run tests without MongoDB (skips repository integration tests)
go test -short ./...

# Run a single test
go test ./internal/repository/ -run TestSubscriberRepository_SaveSubscriber

# Build Docker image
docker build -t book-club-bot .
```

## Configuration

Config is selected by `APP_ENV` env var (`dev` → `config/config_dev.json`, `prod` → `config/config_prod.json`). Defaults to `dev`.

Required env vars:
- `telegrammApiKey` — Telegram Bot API token (can be set in `.env`)
- `APP_ENV` — optional, defaults to `dev`
- `APP_LOCALE` — optional, defaults to `ru` (loads `message/messages_<locale>.json`)

MongoDB connection string and DB name come from the JSON config (`mongo_uri`, `db_name`). Dev config points to `mongodb://localhost:27017`, DB `book_club_boot`.

The Telegram **group ID** is persisted in the MongoDB `settings` collection and loaded at startup. It's set automatically when the bot is added to a group.

## Architecture

```
cmd/main.go           → wires config, messages, MongoDB, repositories, then calls bot.Run()
config/               → JSON-based config per environment
message/              → JSON-based localized UI strings (locale-swappable)
bot/                  → all Telegram bot logic (MongoDB-backed session lifecycle)
internal/
  models/             → MongoDB BSON structs (Subscriber, BookClubSession, etc.)
  repository/         → MongoDB repository layer (SubscriberRepository, SettingsRepository, SessionRepository)
  repository/testing/ → test helpers for spinning up a real test MongoDB
```

### Bot state machine

A book-club round is one **session** persisted in the `book_club_sessions`
collection (`gathering → voting → reading → completed`/`cancelled`). The bot is
**DB-authoritative**: handlers load the active session and write each transition
through `SessionRepository` rather than holding round state in memory, so an
in-flight round survives a restart. At most one session is active at a time,
enforced by a unique partial index (see `docs/book-club-flow.md`).

- **Book gathering** — `/start_vote` creates a session and DMs subscribers; each
  participant's multi-step submission (`book → author → description → image →
  done`, or `skipped`) is persisted on every step.
- **Voting** — a Telegram native poll; votes are recorded with `$addToSet`, and
  the poll closes on its deadline or once all eligible subscribers have voted.
- **Recovery loop** (`bot/recovery.go`) — a single ~15s ticker is the only
  driver of deadlines: it loads the active session, sends due reminders, and
  advances gathering→voting and poll close from the session's persisted
  timestamps. This makes restart-resume identical to normal operation; there are
  no per-deadline `time.Sleep` goroutines.

The main loop in `bot.Run()` dispatches Telegram updates: commands (`/subscribe`,
`/start_vote`, `/skip`, `/help`) to handlers, free-text messages to
`handleUserMsg`, and `PollAnswer` updates to vote counting. Phase transitions are
serialized under `bot.mu`.

### Repository layer

The repositories (`SubscriberRepository`, `SettingsRepository`,
`SessionRepository`) take `*mongo.Database` directly. Lookups return `(nil, nil)`
when not found (e.g. `GetSubscriberById`, `GetActiveSession`). `SessionRepository`
enforces a single active session via a unique partial index on an internal
`activeLock` field; `EnsureIndexes` is called at startup from `cmd/main.go`.

### Testing

Repository tests are integration tests — they connect to a real MongoDB at `localhost:27017`. `TestMain` in `internal/repository/` calls `TestMongoDBConnection()` which exits with code 0 (skipping tests gracefully) if MongoDB is unreachable, unless `-short` is passed in which case the check is skipped entirely. Each test gets a fresh `test_db` database via `CreateTestMongoDB`, which is dropped in the cleanup function.

CI runs MongoDB as a service container (see `.github/workflows/`). Docker image is pushed on git tag.

## Issues and pull requests

Work follows an issue → PR → review flow:

1. **Create an issue** first describing the bug or feature
2. **Implement** on a dedicated branch (`fix/<slug>` or `feat/<slug>`)
3. **Open a PR** that references the issue with `Closes #N` as the first line of the PR body — GitHub will auto-close the issue on merge and link them bidirectionally
4. **Run `/finish-issue`** — triggers an independent code review that posts findings as inline PR comments, then hands off to the human for merge
5. **Address review findings**, then **reply to every inline PR comment** — for each finding, post a reply with the fix commit SHA and a one-line summary of what changed, or an explicit reason why it was skipped
6. **Never push directly to `main`**

Every PR body must follow this structure:

```
Closes #N

## Summary
...

## Test plan
- [ ] ...

---
🤖 Created by [Claude Code](https://claude.ai/code) · Model: claude-sonnet-4-6
```
