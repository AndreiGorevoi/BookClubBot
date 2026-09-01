package bot

// Telegram's length limits for the fields this bot fills, counted the way
// Telegram counts them: in UTF-16 code units, so a character outside the BMP —
// an emoji, say — costs two. Exceeding one of these does not truncate the value,
// it fails the whole API call, so every path that interpolates member-supplied
// text has to bound it against the right limit here.
const (
	telegramCaptionMaxLen = 1024
	telegramMessageMaxLen = 4096
)
