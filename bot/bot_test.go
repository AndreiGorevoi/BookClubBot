package bot

import "testing"

func TestIsAlreadySub(t *testing.T) {
	bot := &Bot{
		subs: []Subscriber{
			{Id: 123, FirstName: "John", LastName: "Doe", Nick: "Jony"},
			{Id: 321, FirstName: "John", LastName: "Doe", Nick: "Jony"},
		},
	}
	data := map[string]struct {
		input    int64
		expected bool
	}{
		`user subscribed`: {
			input:    123,
			expected: true,
		},
		`user is not subscribed`: {
			input:    333,
			expected: false,
		},
	}

	for name, tt := range data {
		t.Run(name, func(t *testing.T) {
			got := bot.isAlreadySub(tt.input)
			if got != tt.expected {
				t.Errorf("expected: %v, got: %v", tt.expected, got)
			}
		})
	}
}
