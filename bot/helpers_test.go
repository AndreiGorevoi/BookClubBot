package bot

import (
	"reflect"
	"sort"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
)

func TestWinningOptionIdx(t *testing.T) {
	data := map[string]struct {
		input    *tgbotapi.Poll
		expected []int
	}{
		`one winner`: {
			input: &tgbotapi.Poll{
				Options: []tgbotapi.PollOption{
					{Text: "Book1", VoterCount: 3},
					{Text: "Book2", VoterCount: 2},
					{Text: "Book3", VoterCount: 1},
				},
			},
			expected: []int{0},
		},
		`two winners`: {
			input: &tgbotapi.Poll{
				Options: []tgbotapi.PollOption{
					{Text: "Book1", VoterCount: 3},
					{Text: "Book2", VoterCount: 3},
					{Text: "Book3", VoterCount: 1},
				},
			},
			expected: []int{0, 1},
		},
		`identical option texts stay separate positions`: {
			input: &tgbotapi.Poll{
				Options: []tgbotapi.PollOption{
					{Text: "Same", VoterCount: 2},
					{Text: "Same", VoterCount: 2},
				},
			},
			expected: []int{0, 1},
		},
		`nobody voted`: {
			input: &tgbotapi.Poll{
				Options: []tgbotapi.PollOption{
					{Text: "Book1", VoterCount: 0},
					{Text: "Book2", VoterCount: 0},
				},
			},
			expected: nil,
		},
		`zero winners`: {
			input:    &tgbotapi.Poll{Options: []tgbotapi.PollOption{}},
			expected: nil,
		},
		`nil imput`: {
			input:    nil,
			expected: nil,
		},
	}

	for name, tt := range data {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, winningOptionIdx(tt.input))
		})
	}
}

func TestShuffleSlice(t *testing.T) {
	t.Run("Empty Slice", func(t *testing.T) {
		s := []int{}
		shuffled := shuffleSlice(s)

		assert.Empty(t, shuffled)
	})

	t.Run("Slice with one element", func(t *testing.T) {
		s := []string{"hello"}
		shuffled := shuffleSlice(s)

		assert.Equal(t, s, shuffled)
	})

	t.Run("Elements preservation", func(t *testing.T) {
		s := []int{1, 2, 3, 4, 5, 6}
		originalSorted := make([]int, len(s))
		copy(originalSorted, s)
		sort.Ints(originalSorted)

		shuffled := shuffleSlice(s)
		shuffledSorted := make([]int, len(shuffled))
		copy(shuffledSorted, shuffled)
		sort.Ints(shuffledSorted)

		assert.Equal(t, originalSorted, shuffledSorted)
	})

	t.Run("name string", func(t *testing.T) {
		s := []int{1, 2, 3, 4, 5, 6}
		original := make([]int, len(s))
		copy(original, s)

		shuffledCount := 0
		numAttemps := 10

		for range make([]struct{}, numAttemps) {
			shuffled := shuffleSlice(s)
			if !reflect.DeepEqual(shuffled, original) {
				shuffledCount++
			}
		}

		assert.NotEqual(t, 0, shuffledCount)
	})

}
