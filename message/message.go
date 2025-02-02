package message

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const folder = "./message"

type LocalizedMessages struct {
	AlreadySubscribedWaitForVoting     string `json:"already_subscribed_wait_for_voting"`
	WelcomeBookClubNextVoting          string `json:"welcome_book_club_next_voting"`
	VotingAlreadyStartedWaitForEnd     string `json:"voting_already_started_wait_for_end"`
	VotingNotStartedOrEnded            string `json:"voting_not_started_or_ended"`
	NotParticipantCurrentVoting        string `json:"not_participant_current_voting"`
	WhoIsAuthor                        string `json:"who_is_author"`
	WriteBookDescription               string `json:"write_book_description"`
	AttachCoverPhoto                   string `json:"attach_cover_photo"`
	BookAddedToNextVoting              string `json:"book_added_to_next_voting"`
	ImageMissingBookAdded              string `json:"image_missing_book_added"`
	VotingAlreadyCompleted             string `json:"voting_already_completed"`
	AlreadyDeclinedSuggestion          string `json:"already_declined_suggestion"`
	UnableToSuggestBook                string `json:"unable_to_suggest_book"`
	PleaseSuggestBookTitle             string `json:"please_suggest_book_title"`
	ErrorDeterminingWinner             string `json:"error_determining_winner"`
	WeHaveAWinner                      string `json:"we_have_a_winner"`
	NoClearWinnerManualVoting          string `json:"no_clear_winner_manual_voting"`
	ChooseUpToTwoBooks                 string `json:"choose_up_to_two_books"`
	BookLabel                          string `json:"book_label"`
	AuthorLabel                        string `json:"author_label"`
	BookSubmissionDeadline             string `json:"book_submission_deadline"`
	VotingEndsInHours                  string `json:"voting_ends_in_hours"`
	CannotStartGatheringGroupIdMissing string `json:"cannot_start_gathering_groupId_missing"`
	BookAlreadyProposed                string `json:"book_already_proposed"`
	HelpInfo                           string `json:"help_info"`
	SomethingWrong                     string `json:"something_wrong"`
	NotSubscriber                      string `json:"not_subscriber"`
	WelcomeBack                        string `json:"welcome_back"`
	Unsubsribed                        string `json:"unsubsribed"`
}

func LoadMessaged() (*LocalizedMessages, error) {
	l := determineLocale()
	return readMessagesFile(l)
}

func determineLocale() string {
	l := os.Getenv("APP_LOCALE")
	if l == "" {
		return "ru" // use ru as default
	}
	return l
}

func readMessagesFile(locale string) (*LocalizedMessages, error) {
	fileName := fmt.Sprintf("messages_%s.json", locale)
	f, err := os.Open(fmt.Sprintf("%s/%s", folder, fileName))
	if err != nil {
		return nil, fmt.Errorf("Cannot open the file: %s", fileName)
	}
	defer f.Close()
	return parseMessaged(f)
}

func parseMessaged(r io.Reader) (*LocalizedMessages, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("Cannot read data from Messages file")
	}

	var res LocalizedMessages
	err = json.Unmarshal(data, &res)
	if err != nil {
		return nil, fmt.Errorf("Cannot unmarshal data during parsing Messages file")
	}
	return &res, nil
}
