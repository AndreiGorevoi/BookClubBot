# Database Schema

MongoDB database: **`book_club_boot`**

---

## Collections in use

### `subscribers`

One document per Telegram user who has ever subscribed to the bot. The Telegram user ID is the primary key.

```json
{
  "_id": 123456789,
  "firstName": "Andrei",
  "lastName": "Haravy",
  "nick": "andreiharavy",
  "archived": false,
  "joinedAt": "2025-01-04T10:00:00Z"
}
```

| Field | BSON type | Notes |
|---|---|---|
| `_id` | int64 | Telegram user ID |
| `firstName` | string | |
| `lastName` | string | |
| `nick` | string | Telegram username (without `@`) |
| `archived` | bool | `true` = unsubscribed; user can resubscribe, record is kept |
| `joinedAt` | date | Timestamp of initial subscription |

**Operations:** upsert on save, `$set archived` on subscribe/unsubscribe, full-collection scan for `GetAllSubscribers` (used to build participant lists for a vote round).

**No indexes defined** beyond the default `_id` index.

---

### `settings`

Single-document collection. Stores bot-level runtime settings that must survive restarts.

```json
{
  "_id": "settings",
  "groupId": -1001234567890
}
```

| Field | BSON type | Notes |
|---|---|---|
| `_id` | string | Hard-coded to `"settings"` — always a single document |
| `groupId` | int64 | Telegram group chat ID. Set when the bot is added to a group, reset to `0` when removed. `0` means no active group. |

**Operations:** upsert on write, single FindOne on read. Read once at bot startup to restore the active group.

---

## Session models — earlier draft (historical)

> **Superseded.** The `book_club_sessions` collection is now live, but the schema
> sketched below is an **earlier draft** and no longer matches the code. The
> authoritative design for the book-club round (gathering → voting → reading) and
> its MongoDB schema lives in [`book-club-flow.md`](./book-club-flow.md). The
> section below is kept only for historical reference.

The following describes the original draft structs in `internal/models/mongodb.go`. They have since been reworked and persisted via `SessionRepository`; see `book-club-flow.md` for the current shape.

### `BookClubSession` (planned collection: `book_club_sessions`)

Top-level document for one complete voting round — from opening book suggestions through to the winner announcement.

```json
{
  "_id": "<ObjectID>",
  "name": "June 2025 Session",
  "status": "open",
  "startedAt": "2025-06-01T10:00:00Z",
  "createdBy": 123456789,
  "bookSuggestions": [
    {
      "subscriberId": 123456789,
      "bookTitle": "The Pragmatic Programmer",
      "author": "David Thomas",
      "description": "A classic on software craftsmanship.",
      "photoId": "AgACAgIAAxk...",
      "suggestedAt": "2025-06-01T11:00:00Z"
    }
  ],
  "voting": {
    "telegramPollId": "42",
    "startedAt": "2025-06-02T10:00:00Z",
    "completedAt": null,
    "participantsVoted": 3,
    "totalParticipants": 5
  },
  "winner": {
    "bookTitle": "The Pragmatic Programmer",
    "author": "David Thomas",
    "description": "A classic on software craftsmanship.",
    "photoId": "AgACAgIAAxk...",
    "subscriberId": 123456789
  }
}
```

**`BookClubSession` fields:**

| Field | BSON type | Notes |
|---|---|---|
| `_id` | ObjectID | Auto-generated |
| `name` | string | Human-readable session label |
| `status` | string | Intended values: `open`, `voting`, `completed` (not enforced in code yet) |
| `startedAt` | date | When the book-gathering phase began |
| `createdBy` | int64 | Telegram user ID of whoever ran `/start_vote` |
| `bookSuggestions` | array | Embedded `BookSuggestion` objects (one per participating subscriber) |
| `voting` | object | Embedded `Voting` sub-document |
| `winner` | object | Embedded `Winner` sub-document (populated after poll closes) |

**`BookSuggestion` (embedded array element):**

| Field | BSON type | Notes |
|---|---|---|
| `subscriberId` | int64 | References `subscribers._id` |
| `bookTitle` | string | |
| `author` | string | |
| `description` | string | |
| `photoId` | string | Telegram `FileID`; empty string if no photo was submitted |
| `suggestedAt` | date | |

**`Voting` (embedded sub-document):**

| Field | BSON type | Notes |
|---|---|---|
| `telegramPollId` | string | Telegram message ID of the native poll (stored as string) |
| `startedAt` | date | |
| `completedAt` | date \| null | `null` while the poll is still open |
| `participantsVoted` | int32 | Running count of unique voters |
| `totalParticipants` | int32 | Snapshot of active subscriber count at poll start |

**`Winner` (embedded sub-document):**

| Field | BSON type | Notes |
|---|---|---|
| `bookTitle` | string | Copied from the winning `BookSuggestion` |
| `author` | string | |
| `description` | string | |
| `photoId` | string | |
| `subscriberId` | int64 | Who suggested the winning book |

---

## Current vs. planned state

| Collection | Status | Notes |
|---|---|---|
| `subscribers` | **Live** | Full CRUD via `SubscriberRepository` |
| `settings` | **Live** | Single-document store for `groupId` |
| `book_club_sessions` | **Live** | Full lifecycle via `SessionRepository`; the bot is DB-authoritative and resumes in-flight rounds after a restart. Schema and behavior: [`book-club-flow.md`](./book-club-flow.md). |
