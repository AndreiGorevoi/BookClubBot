package bot

import (
	"BookClubBot/internal/models"
	"BookClubBot/message"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOnboardingKeyboard(t *testing.T) {
	b := &Bot{messages: &message.LocalizedMessages{
		BtnOnboardingSkip: "Skip",
	}}

	kb := b.onboardingKeyboard()
	skip := kb.InlineKeyboard[0][0]

	assert.Equal(t, "Skip", skip.Text)
	assert.NotNil(t, skip.CallbackData)
	assert.Equal(t, callbackOnboardingSkip, *skip.CallbackData)
}

func TestSubscriberIsOnboarding(t *testing.T) {
	assert.False(t, (&models.Subscriber{}).IsOnboarding())
	assert.True(t, (&models.Subscriber{OnboardingStep: models.OnboardingGenres}).IsOnboarding())
}
