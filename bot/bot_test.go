package bot

import (
	"slices"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestDefineWinners(t *testing.T) {
	data := map[string]struct {
		input           *tgbotapi.Poll
		expectedWinners []string
		expectedLen     int
	}{
		`one winner`: {
			input: &tgbotapi.Poll{
				Options: []tgbotapi.PollOption{
					{Text: "Book1", VoterCount: 3},
					{Text: "Book2", VoterCount: 2},
					{Text: "Book3", VoterCount: 1},
				},
			},
			expectedWinners: []string{"Book1"},
			expectedLen:     1,
		},
		`two winners`: {
			input: &tgbotapi.Poll{
				Options: []tgbotapi.PollOption{
					{Text: "Book1", VoterCount: 3},
					{Text: "Book2", VoterCount: 3},
					{Text: "Book3", VoterCount: 1},
				},
			},
			expectedWinners: []string{"Book1", "Book2"},
			expectedLen:     2,
		},
		`zero winners`: {
			input: &tgbotapi.Poll{
				Options: []tgbotapi.PollOption{},
			},
			expectedWinners: []string{},
			expectedLen:     0,
		},
		`nil imput`: {
			input:           nil,
			expectedWinners: []string{},
			expectedLen:     0,
		},
	}

	for name, tt := range data {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := defineWinners(tt.input)
			if len(got) != tt.expectedLen {
				t.Errorf("expected len: %d, got: %d", tt.expectedLen, len(got))
			}
			for _, s := range got {
				if !slices.Contains(tt.expectedWinners, s) {
					t.Errorf("expected winners: %v doesn't containt %s", tt.expectedWinners, s)
				}
			}

		})
	}
}
