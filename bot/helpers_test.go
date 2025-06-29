package bot

import (
	"reflect"
	"slices"
	"sort"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
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

func TestSplitMedia(t *testing.T) {
	data := map[string]struct {
		participants    []*participant
		batchSize       int
		expectedBatches int
	}{
		`single participant with book, batchSize 1`: {
			participants: []*participant{
				{
					id:        1,
					firstName: "Alice",
					lastName:  "Smith",
					book: &book{
						title:       "Book1",
						author:      "Author1",
						description: "Description1",
						photoId:     "photoId1",
					},
				},
			},
			batchSize:       1,
			expectedBatches: 1,
		},
		`multiple participants with books, exact batch size`: {
			participants: []*participant{
				{
					id:        1,
					firstName: "Alice",
					lastName:  "Smith",
					book: &book{
						title:       "Book1",
						author:      "Author1",
						description: "Description1",
						photoId:     "photoId1",
					},
				},
				{
					id:        2,
					firstName: "Bob",
					lastName:  "Brown",
					book: &book{
						title:       "Book2",
						author:      "Author2",
						description: "Description2",
						photoId:     "photoId2",
					},
				},
			},
			batchSize:       2,
			expectedBatches: 1,
		},
		`multiple participants with books, batch size smaller than participants`: {
			participants: []*participant{
				{
					id:        1,
					firstName: "Alice",
					lastName:  "Smith",
					book: &book{
						title:       "Book1",
						author:      "Author1",
						description: "Description1",
						photoId:     "photoId1",
					},
				},
				{
					id:        2,
					firstName: "Bob",
					lastName:  "Brown",
					book: &book{
						title:       "Book2",
						author:      "Author2",
						description: "Description2",
						photoId:     "photoId2",
					},
				},
				{
					id:        3,
					firstName: "Charlie",
					lastName:  "Davis",
					book: &book{
						title:       "Book3",
						author:      "Author3",
						description: "Description3",
						photoId:     "photoId3",
					},
				},
			},
			batchSize:       2,
			expectedBatches: 2,
		},
		`participants without books are ignored`: {
			participants: []*participant{
				{
					id:        1,
					firstName: "Alice",
					lastName:  "Smith",
					book:      nil,
				},
				{
					id:        2,
					firstName: "Bob",
					lastName:  "Brown",
					book: &book{
						title:       "Book2",
						author:      "Author2",
						description: "Description2",
						photoId:     "photoId2",
					},
				},
			},
			batchSize:       1,
			expectedBatches: 1,
		},
		`empty participant list`: {
			participants:    []*participant{},
			batchSize:       1,
			expectedBatches: 0,
		},
		`batch size greater than participants`: {
			participants: []*participant{
				{
					id:        1,
					firstName: "Alice",
					lastName:  "Smith",
					book: &book{
						title:       "Book1",
						author:      "Author1",
						description: "Description1",
						photoId:     "photoId1",
					},
				},
			},
			batchSize:       5,
			expectedBatches: 1,
		},
	}

	for name, tt := range data {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := splitMedia(tt.participants, tt.batchSize)
			if len(got) != tt.expectedBatches {
				t.Errorf("expected batches: %d, got: %d", tt.expectedBatches, len(got))
			}
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
