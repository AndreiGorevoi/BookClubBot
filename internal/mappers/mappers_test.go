package mappers

import (
	"BookClubBot/internal/models"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSubscriberToParticipant(t *testing.T) {
	t.Run("valid subscriber conversion", func(t *testing.T) {
		// Arrange
		subscriber := &models.Subscriber{
			ID:        123,
			FirstName: "John",
			LastName:  "Doe",
			Nick:      "JohnnyD",
			Archived:  false,
			JoinedAt:  time.Now(),
		}

		expectedParticipant := models.Participant{
			ID:       subscriber.ID,
			NickName: subscriber.Nick,
			Status:   0,
			// Book field is not set by the mapper, so it should be its zero value (empty struct)
		}

		// Act
		participant := subscriberToParticipant(subscriber)

		// Assert
		assert.Equal(t, expectedParticipant.ID, participant.ID)
		assert.Equal(t, expectedParticipant.NickName, participant.NickName)
		assert.Equal(t, expectedParticipant.Status, participant.Status)
		assert.Equal(t, expectedParticipant.Book, participant.Book) // Ensure Book is its zero value
	})

	t.Run("subscriber with empty nick", func(t *testing.T) {
		// Arrange
		subscriber := &models.Subscriber{
			ID:   456,
			Nick: "", // Empty Nick
		}

		expectedParticipant := models.Participant{
			ID:       subscriber.ID,
			NickName: "",
			Status:   0,
		}

		// Act
		participant := subscriberToParticipant(subscriber)

		// Assert
		assert.Equal(t, expectedParticipant.ID, participant.ID)
		assert.Equal(t, expectedParticipant.NickName, participant.NickName)
		assert.Equal(t, expectedParticipant.Status, participant.Status)
	})

	t.Run("subscriber with large ID", func(t *testing.T) {
		// Arrange
		subscriber := &models.Subscriber{
			ID:   9876543210, // Large int64 ID
			Nick: "BigIDUser",
		}

		expectedParticipant := models.Participant{
			ID:       subscriber.ID,
			NickName: subscriber.Nick,
			Status:   0,
		}

		// Act
		participant := subscriberToParticipant(subscriber)

		// Assert
		assert.Equal(t, expectedParticipant.ID, participant.ID)
		assert.Equal(t, expectedParticipant.NickName, participant.NickName)
		assert.Equal(t, expectedParticipant.Status, participant.Status)
	})
}

func TestSubscribersToParticipants(t *testing.T) {
	t.Run("empty subscribers slice", func(t *testing.T) {
		// Arrange
		var subs []*models.Subscriber

		// Act
		res := SubscribersToParticipants(subs)

		// Assert
		assert.NotNil(t, res)
		assert.Empty(t, res)
		assert.Len(t, res, 0)
	})

	t.Run("single subscriber in slice", func(t *testing.T) {
		// Arrange
		subscriber := &models.Subscriber{
			ID:   1,
			Nick: "Alice",
		}
		subs := []*models.Subscriber{subscriber}

		expectedParticipant := models.Participant{
			ID:       subscriber.ID,
			NickName: subscriber.Nick,
			Status:   0,
		}

		// Act
		res := SubscribersToParticipants(subs)
		fmt.Println(res)

		// Assert
		assert.Len(t, res, 1)
		assert.Equal(t, expectedParticipant.ID, res[0].ID)
		assert.Equal(t, expectedParticipant.NickName, res[0].NickName)
		assert.Equal(t, expectedParticipant.Status, res[0].Status)
	})

	t.Run("multiple subscribers in slice", func(t *testing.T) {
		// Arrange
		subscriber1 := &models.Subscriber{ID: 101, Nick: "Bob"}
		subscriber2 := &models.Subscriber{ID: 102, Nick: "Charlie"}
		subs := []*models.Subscriber{subscriber1, subscriber2}

		expectedParticipant1 := models.Participant{
			ID:       subscriber1.ID,
			NickName: subscriber1.Nick,
			Status:   0,
		}
		expectedParticipant2 := models.Participant{
			ID:       subscriber2.ID,
			NickName: subscriber2.Nick,
			Status:   0,
		}

		// Act
		res := SubscribersToParticipants(subs)

		// Assert
		assert.Len(t, res, 2)
		assert.Equal(t, expectedParticipant1.ID, res[0].ID)
		assert.Equal(t, expectedParticipant1.NickName, res[0].NickName)
		assert.Equal(t, expectedParticipant1.Status, res[0].Status)

		assert.Equal(t, expectedParticipant2.ID, res[1].ID)
		assert.Equal(t, expectedParticipant2.NickName, res[1].NickName)
		assert.Equal(t, expectedParticipant2.Status, res[1].Status)
	})

	t.Run("subscribers with varying data", func(t *testing.T) {
		// Arrange
		subscriber1 := &models.Subscriber{ID: 201, Nick: "Dave"}
		subscriber2 := &models.Subscriber{ID: 202, Nick: ""} // Empty nick
		subscriber3 := &models.Subscriber{ID: 203, Nick: "Eve"}
		subs := []*models.Subscriber{subscriber1, subscriber2, subscriber3}

		expectedParticipant1 := models.Participant{ID: subscriber1.ID, NickName: "Dave", Status: 0}
		expectedParticipant2 := models.Participant{ID: subscriber2.ID, NickName: "", Status: 0}
		expectedParticipant3 := models.Participant{ID: subscriber3.ID, NickName: "Eve", Status: 0}

		// Act
		res := SubscribersToParticipants(subs)

		// Assert
		assert.Len(t, res, 3)
		assert.Equal(t, expectedParticipant1, res[0])
		assert.Equal(t, expectedParticipant2, res[1])
		assert.Equal(t, expectedParticipant3, res[2])
	})
}
