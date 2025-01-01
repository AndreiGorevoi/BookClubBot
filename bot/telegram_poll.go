package bot

type telegramPoll struct {
	pollId       int
	participants int
	voted        map[int64]struct{}
	isActive     bool
}
