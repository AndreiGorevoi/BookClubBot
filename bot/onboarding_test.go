package bot

import (
	"BookClubBot/internal/models"
	"BookClubBot/message"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOnboardingKeyboard(t *testing.T) {
	b := &Bot{messages: &message.LocalizedMessages{
		BtnOnboardingSkipQuestion: "Skip question",
		BtnOnboardingSkipAll:      "Skip intro",
	}}

	kb := b.onboardingKeyboard()
	skipQ := kb.InlineKeyboard[0][0]
	skipAll := kb.InlineKeyboard[0][1]

	assert.Equal(t, "Skip question", skipQ.Text)
	assert.NotNil(t, skipQ.CallbackData)
	assert.Equal(t, callbackOnboardingSkipQuestion, *skipQ.CallbackData)

	assert.Equal(t, "Skip intro", skipAll.Text)
	assert.NotNil(t, skipAll.CallbackData)
	assert.Equal(t, callbackOnboardingSkipAll, *skipAll.CallbackData)
}

func TestNextOnboardingStep(t *testing.T) {
	assert.Equal(t, models.OnboardingFavBook, nextOnboardingStep(models.OnboardingGenres))
	assert.Equal(t, models.OnboardingFunFact, nextOnboardingStep(models.OnboardingFavBook))
	assert.Equal(t, "", nextOnboardingStep(models.OnboardingFunFact))
	assert.Equal(t, "", nextOnboardingStep("")) // unexpected/empty → done
}

func TestApplyOnboardingAnswer(t *testing.T) {
	t.Run("answer lands in the field for the current step", func(t *testing.T) {
		s := &models.Subscriber{OnboardingStep: models.OnboardingGenres}
		applyOnboardingAnswer(s, "fantasy, sci-fi")
		assert.Equal(t, "fantasy, sci-fi", s.FavoriteGenres)
		assert.Empty(t, s.FavoriteBook)
		assert.Empty(t, s.FunFact)

		s.OnboardingStep = models.OnboardingFavBook
		applyOnboardingAnswer(s, "Dune")
		assert.Equal(t, "Dune", s.FavoriteBook)

		s.OnboardingStep = models.OnboardingFunFact
		applyOnboardingAnswer(s, "I can juggle")
		assert.Equal(t, "I can juggle", s.FunFact)
	})

	t.Run("no step means nothing is written", func(t *testing.T) {
		s := &models.Subscriber{}
		applyOnboardingAnswer(s, "ignored")
		assert.Empty(t, s.FavoriteGenres)
		assert.Empty(t, s.FavoriteBook)
		assert.Empty(t, s.FunFact)
	})
}

func TestSubscriberIsOnboarding(t *testing.T) {
	assert.False(t, (&models.Subscriber{}).IsOnboarding())
	assert.True(t, (&models.Subscriber{OnboardingStep: models.OnboardingGenres}).IsOnboarding())
}
