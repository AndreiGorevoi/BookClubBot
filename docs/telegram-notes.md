# Telegram Bot API — working notes

Three sources, in order of authority:

1. **The website** — <https://core.telegram.org/bots/api>. Always right.
2. **`docs/vendor/telegram-bot-api.md`** — an offline Markdown mirror of that
   page, so limits and parameter tables can be grepped without a network round
   trip. Refresh with `python3 scripts/fetch_telegram_docs.py`.
3. **The Go wrapper's source** — `$(go env GOMODCACHE)/github.com/go-telegram-bot-api/telegram-bot-api/v5@v5.5.1/`
   (`types.go` for structs, `configs.go` for request params, `helpers.go` for
   the `New*` constructors). This is the authority on what this project can
   actually *call*.

## Mind the version gap

The mirror documents **Bot API 10.3**; the pinned wrapper `v5.5.1` implements
roughly **Bot API 5.5** (it has `ChatJoinRequest` and `HasProtectedContent`
from 5.4, but no `WebAppData` from 6.0). A field can be perfectly documented
and still not exist in the library. Before using one:

```bash
grep -n "FieldName" "$(go env GOMODCACHE)"/github.com/go-telegram-bot-api/telegram-bot-api/v5@v5.5.1/types.go
```

## Limits this bot touches

All string limits are counted in UTF-16 code units, so an emoji costs two.
Exceeding one does not truncate — the whole API call fails.

| Thing | Limit | Where |
|---|---|---|
| Message text | 1–4096 | `sendMessage` |
| Media caption | 0–1024 | `sendPhoto` et al. |
| `callback_data` | 1–64 **bytes** | `InlineKeyboardButton` |
| `answerCallbackQuery` text | 0–200 | shown as a toast, or an alert with `show_alert` |
| Poll question | 1–300 | `sendPoll` |
| Poll option text | 1–100 | `sendPoll` |
| Poll options count | 1–12 | `sendPoll` |
| Broadcast rate | 30 messages/second free | see also the [FAQ](https://core.telegram.org/bots/faq#my-bot-is-hitting-limits-how-do-i-avoid-this): ~20 messages/minute into one group |

`bot/limits.go` holds the two the bot bounds text against; issues #47, #53
and #55 were all this table being violated one way or another.

## Behaviour that isn't obvious from the tables

- **Editing a message to identical content fails.** `editMessageText` /
  `editMessageReplyMarkup` return `400 Bad Request: message is not modified` when both
  the text and the markup are unchanged. Single-message panels have to either
  diff before editing or treat that specific error as success. (Observed
  behaviour, not stated in the API docs.)
- **`editMessageText` replaces the keyboard.** Omitting `reply_markup` in the
  edit drops the buttons; pass the markup every time you want them kept.
- **Inline buttons outlive the state they were drawn for.** They stay in chat
  history forever, so every callback has to re-check authorization and
  re-validate the current step rather than trusting that the button was only
  drawn when the action was legal. See `handleCallback` in `bot/bot.go`.
- **Always answer the callback query.** Until `answerCallbackQuery` lands the
  client keeps showing a spinner on the button; an empty text just stops it.
- **`parse_mode` makes user text dangerous.** Unescaped `_ * [ ] ( ) ~ ` > # +
  - = | { } . !` breaks MarkdownV2, and `< > &` breaks HTML — a malformed
  entity fails the send. This bot uses `Markdown` (`bot/bot.go`) and `HTML`
  (`bot/reminders.go`); escaping rules are under *Formatting options* in the
  mirror. Issue #55 came from exactly this.
- **A poll's `PollAnswer` update carries no chat and no option text.** A vote
  arrives as a poll id, a user id and option *indexes*, so anything about the
  option has to be resolved by position against what was sent — that is what
  `Voting.OptionOwners` is for (issue #53).
