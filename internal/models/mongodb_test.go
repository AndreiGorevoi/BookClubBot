package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
)

// Regression for the "stuck onboarding" bug: OnboardingStep must NOT carry
// `omitempty`. SaveSubscriber persists the whole struct via `$set`, and with
// omitempty an empty step is dropped from the update — so finishing onboarding
// (step -> "") would never reach the DB and the subscriber would re-trigger the
// onboarding flow on every subsequent message.
func TestSubscriber_OnboardingStepIsPersistedWhenEmpty(t *testing.T) {
	data, err := bson.Marshal(&Subscriber{ID: 1, OnboardingStep: ""})
	assert.NoError(t, err)

	var doc bson.M
	assert.NoError(t, bson.Unmarshal(data, &doc))

	_, present := doc["onboardingStep"]
	assert.True(t, present, "onboardingStep must marshal even when empty so it can be cleared")
}

func TestSubscriber_IsOnboarding(t *testing.T) {
	assert.False(t, (&Subscriber{}).IsOnboarding())
	assert.True(t, (&Subscriber{OnboardingStep: OnboardingGenres}).IsOnboarding())
}
