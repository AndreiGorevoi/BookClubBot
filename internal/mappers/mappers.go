package mappers

import (
	"BookClubBot/internal/models"
)

func subscriberToParticipant(subscriber *models.Subscriber) models.Participant {
	var participant models.Participant
	participant.ID = subscriber.ID
	participant.NickName = subscriber.Nick
	participant.Status = 0
	return participant
}

func SubscribersToParticipants(subs []*models.Subscriber) []models.Participant {
	participants := make([]models.Participant, 0, len(subs))

	for _, s := range subs {
		p := subscriberToParticipant(s)
		participants = append(participants, p)
	}

	return participants
}
