package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShippedConfigsResolve guards the config files that actually ship: a typo
// in a timezone name or an out-of-range quiet hour is otherwise only discovered
// when the bot fails to start, which for prod means after a deploy.
func TestShippedConfigsResolve(t *testing.T) {
	// dev deliberately runs without quiet hours: its gathering window is 60
	// seconds, and a nine-hour nightly silence would make the feature untestable
	// locally after 23:00. sandbox mirrors prod so it stays a faithful rehearsal.
	quietHoursOn := map[string]bool{"dev": false, "sandbox": true, "prod": true}

	for env, wantQuietHours := range quietHoursOn {
		t.Run(env, func(t *testing.T) {
			f, err := os.Open("config_" + env + ".json")
			require.NoError(t, err)
			defer f.Close()

			cfg, err := parsreAppConfig(f)
			require.NoError(t, err)

			require.NoError(t, cfg.resolveQuietHours())
			assert.Equal(t, wantQuietHours, cfg.Location != nil)

			assert.Positive(t, cfg.TimeToGatherBooks)
			assert.Positive(t, cfg.GatheringReminderInterval)
			// A reminder is only scheduled at deadline-k*interval for a point that
			// falls strictly inside the window, so an interval at least as long as
			// the gathering itself would silently disable reminders.
			assert.Less(t, cfg.GatheringReminderInterval, cfg.TimeToGatherBooks,
				"interval must be shorter than the gathering window or no reminder ever fires")
		})
	}
}

func TestResolveQuietHours(t *testing.T) {
	t.Run("empty timezone disables quiet hours", func(t *testing.T) {
		cfg := &AppConfig{QuietHoursStart: 23, QuietHoursEnd: 8}
		require.NoError(t, cfg.resolveQuietHours())
		assert.Nil(t, cfg.Location)
	})

	t.Run("unknown timezone fails loudly", func(t *testing.T) {
		// Falling back to UTC would silence reminders during the wrong nine hours
		// of the day, which is worse than refusing to start.
		cfg := &AppConfig{QuietHoursStart: 23, QuietHoursEnd: 8, Timezone: "Europe/Nowhere"}
		assert.Error(t, cfg.resolveQuietHours())
	})

	t.Run("out of range hours are rejected", func(t *testing.T) {
		cfg := &AppConfig{QuietHoursStart: 23, QuietHoursEnd: 24, Timezone: "UTC"}
		assert.Error(t, cfg.resolveQuietHours())

		cfg = &AppConfig{QuietHoursStart: -1, QuietHoursEnd: 8, Timezone: "UTC"}
		assert.Error(t, cfg.resolveQuietHours())
	})
}
