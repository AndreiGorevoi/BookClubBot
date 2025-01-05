package bot

import (
	"testing"
)

func TestRemoveParticipant(t *testing.T) {
	bg := &bookGathering{
		participants: []*participant{
			{id: 1},
			{id: 2},
			{id: 3},
		},
	}
	userDoDelete := int64(2)
	bg.removeParticipant(userDoDelete)

	for _, p := range bg.participants {
		if p.id == userDoDelete {
			t.Errorf("expected: %d was removed. got: it is still there", userDoDelete)
		}
	}
}
