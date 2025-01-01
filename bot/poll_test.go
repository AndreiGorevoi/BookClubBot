package bot

import (
	"testing"
)

func TestRemoveParticipant(t *testing.T) {
	bg := &BookGathering{
		Participants: []*Participant{
			{Id: 1},
			{Id: 2},
			{Id: 3},
		},
	}
	userDoDelete := int64(2)
	bg.removeParticipant(userDoDelete)

	for _, p := range bg.Participants {
		if p.Id == userDoDelete {
			t.Errorf("expected: %d was removed. got: it is still there", userDoDelete)
		}
	}
}
