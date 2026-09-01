package bot

import (
	"math/rand"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// winningOptionIdx returns the positions of the poll options that tied for the
// most votes. A poll nobody voted in has no winner, so it returns nothing rather
// than reporting every option as a winner.
func winningOptionIdx(res *tgbotapi.Poll) []int {
	if res == nil {
		return nil
	}
	max := 0
	for _, o := range res.Options {
		if o.VoterCount > max {
			max = o.VoterCount
		}
	}
	if max == 0 {
		return nil
	}

	idx := make([]int, 0, len(res.Options))
	for i, o := range res.Options {
		if o.VoterCount == max {
			idx = append(idx, i)
		}
	}
	return idx
}

// escapeMarkdown neutralises the legacy-Markdown metacharacters in text the
// members wrote. Without it a single unpaired '_', '*', '`' or '[' — a footnote
// marker pasted in from a web page, say — makes Telegram reject the whole send
// with "can't parse entities". Mirrors escapeHTML in reminders.go.
func escapeMarkdown(s string) string {
	return tgbotapi.EscapeText(tgbotapi.ModeMarkdown, s)
}

// elide cuts s to at most limit UTF-16 units, marking the cut with an ellipsis
// so a reader can tell text was dropped.
func elide(s string, limit int) string {
	if utf16Len(s) <= limit {
		return s
	}
	if limit <= 0 {
		return ""
	}
	return truncateUTF16(s, limit-1) + "\u2026"
}

// elideEscaped is elide for text that has already been escaped for Markdown: the
// cut must not leave a dangling backslash, which would escape the ellipsis that
// follows it and break the very parsing the escaping protects.
func elideEscaped(s string, limit int) string {
	if utf16Len(s) <= limit {
		return s
	}
	if limit <= 0 {
		return ""
	}
	return dropDanglingEscape(truncateUTF16(s, limit-1)) + "\u2026"
}

// dropDanglingEscape removes a trailing backslash left unpaired by a cut.
func dropDanglingEscape(s string) string {
	trailing := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		trailing++
	}
	if trailing%2 == 1 {
		return s[:len(s)-1]
	}
	return s
}

// fitFields shrinks fields, in the order given, until their combined length fits
// budget UTF-16 units. Only the fields give way — never the template around them,
// whose markup must stay intact — so callers pass the most expendable field
// first. Nothing is shortened when it all fits already.
func fitFields(budget int, elideFn func(string, int) string, fields ...*string) {
	total := 0
	for _, f := range fields {
		total += utf16Len(*f)
	}
	for _, f := range fields {
		if total <= budget {
			return
		}
		was := utf16Len(*f)
		*f = elideFn(*f, was-(total-budget))
		total -= was - utf16Len(*f)
	}
}

// utf16Len reports the length of s the way Telegram measures text: in UTF-16
// code units, so a character outside the BMP (an emoji, say) counts as two.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// truncateUTF16 cuts s down to at most limit UTF-16 code units. It never splits
// a character: one that would straddle the limit is dropped whole.
func truncateUTF16(s string, limit int) string {
	n := 0
	for i, r := range s {
		w := 1
		if r > 0xFFFF {
			w = 2
		}
		if n+w > limit {
			return s[:i]
		}
		n += w
	}
	return s
}

func shuffleSlice[T any](s []T) []T {
	copyS := make([]T, len(s))
	copy(copyS, s)
	rand.Shuffle(len(copyS), func(i, j int) {
		copyS[i], copyS[j] = copyS[j], copyS[i]
	})
	return copyS
}
