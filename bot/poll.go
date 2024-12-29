package bot

const (
	INIT = iota
	BOOK_IS_ASKED
	FINISHED
)

type BookPoll struct {
	Participants []*Participant
	IsStarted    bool
}

type Participant struct {
	Id        int64
	FirstName string
	LastName  string
	Nick      string
	Status    int
	Book      *Book
}

type Book struct {
	Title       string
	Author      string
	Description string
}
