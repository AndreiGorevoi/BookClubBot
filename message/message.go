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
	NotEnoughBooksVotingCancelled      string `json:"not_enough_books_voting_cancelled"`
	BookLabel                          string `json:"book_label"`
	AuthorLabel                        string `json:"author_label"`
	BookSubmissionDeadline             string `json:"book_submission_deadline"`
	GatheringReminder                  string `json:"gathering_reminder"`
	TimeLeftHours                      string `json:"time_left_hours"`
	TimeLeftMinutes                    string `json:"time_left_minutes"`
	VotingEndsInHours                  string `json:"voting_ends_in_hours"`
	CannotStartGatheringGroupIdMissing string `json:"cannot_start_gathering_groupId_missing"`
	StartVoteAdminOnly                 string `json:"start_vote_admin_only"`
	BookAlreadyProposed                string `json:"book_already_proposed"`
	HelpInfo                           string `json:"help_info"`
	SomethingWrong                     string `json:"something_wrong"`
	NotSubscriber                      string `json:"not_subscriber"`
	WelcomeBack                        string `json:"welcome_back"`
	Unsubsribed                        string `json:"unsubsribed"`
	GreetingMessage                    string `json:"greeting_message"`
	BtnSkipGathering                   string `json:"btn_skip_gathering"`
	BtnNoCover                         string `json:"btn_no_cover"`
	BookReviewSummary                  string `json:"book_review_summary"`
	CoverAttached                      string `json:"cover_attached"`
	CoverMissing                       string `json:"cover_missing"`
	BtnConfirmBook                     string `json:"btn_confirm_book"`
	BtnRestartBook                     string `json:"btn_restart_book"`
	OnboardingIntro                    string `json:"onboarding_intro"`
	OnboardingAskGenres                string `json:"onboarding_ask_genres"`
	OnboardingDone                     string `json:"onboarding_done"`
	BtnOnboardingSkip                  string `json:"btn_onboarding_skip"`

	// Admin console (bot/admin.go).
	AdminOnly                string `json:"admin_only"`
	AdminPanelTitle          string `json:"admin_panel_title"`
	AdminStatusLine          string `json:"admin_status_line"`
	AdminStatusUnknown       string `json:"admin_status_unknown"`
	AdminNoActiveSession     string `json:"admin_no_active_session"`
	AdminPhaseGathering      string `json:"admin_phase_gathering"`
	AdminPhaseVoting         string `json:"admin_phase_voting"`
	AdminPhaseReading        string `json:"admin_phase_reading"`
	AdminBtnMembers          string `json:"admin_btn_members"`
	AdminBtnSession          string `json:"admin_btn_session"`
	AdminBtnUnsubscribe      string `json:"admin_btn_unsubscribe"`
	AdminBtnEndRound         string `json:"admin_btn_end_round"`
	AdminBtnBack             string `json:"admin_btn_back"`
	AdminBtnPrev             string `json:"admin_btn_prev"`
	AdminBtnNext             string `json:"admin_btn_next"`
	AdminMembersTitle        string `json:"admin_members_title"`
	AdminMembersEmpty        string `json:"admin_members_empty"`
	AdminMembersPage         string `json:"admin_members_page"`
	AdminSessionHeader       string `json:"admin_session_header"`
	AdminSessionDeadline     string `json:"admin_session_deadline"`
	AdminSessionDeadlinePast string `json:"admin_session_deadline_past"`
	AdminSessionSubmitted    string `json:"admin_session_submitted"`
	AdminSessionPending      string `json:"admin_session_pending"`
	AdminSessionSkipped      string `json:"admin_session_skipped"`
	AdminSessionVotes        string `json:"admin_session_votes"`
	AdminNobody              string `json:"admin_nobody"`
	AdminEndConfirm          string `json:"admin_end_confirm"`
	AdminBtnEndConfirm       string `json:"admin_btn_end_confirm"`
	AdminEndDone             string `json:"admin_end_done"`
	AdminRoundCancelledGroup string `json:"admin_round_cancelled_group"`
	AdminUnsubTitle          string `json:"admin_unsub_title"`
	AdminUnsubConfirm        string `json:"admin_unsub_confirm"`
	AdminBtnUnsubConfirm     string `json:"admin_btn_unsub_confirm"`
	AdminUnsubDone           string `json:"admin_unsub_done"`
	AdminUnsubGone           string `json:"admin_unsub_gone"`
	AdminActionFailed        string `json:"admin_action_failed"`
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
