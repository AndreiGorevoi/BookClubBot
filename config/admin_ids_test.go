package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAdminIDs(t *testing.T) {
	t.Run("comma separated with whitespace and empty entries", func(t *testing.T) {
		ids, err := parseAdminIDs(" 123, 456 ,,789,")
		require.NoError(t, err)
		assert.Equal(t, []int64{123, 456, 789}, ids)
	})

	t.Run("negative ids are accepted", func(t *testing.T) {
		// Telegram user ids are positive, but the parser should not be the
		// place that decides that; it only has to be a valid int64.
		ids, err := parseAdminIDs("-1")
		require.NoError(t, err)
		assert.Equal(t, []int64{-1}, ids)
	})

	t.Run("non-numeric entry fails", func(t *testing.T) {
		_, err := parseAdminIDs("123,@someone")
		assert.Error(t, err)
	})

	t.Run("blank input yields no ids", func(t *testing.T) {
		ids, err := parseAdminIDs("  ")
		require.NoError(t, err)
		assert.Empty(t, ids)
	})
}

func TestApplyAdminIDsEnv(t *testing.T) {
	t.Run("unset env keeps the JSON list", func(t *testing.T) {
		cfg := &AppConfig{AdminIDs: []int64{1, 2}}
		require.NoError(t, cfg.applyAdminIDsEnv(""))
		assert.Equal(t, []int64{1, 2}, cfg.AdminIDs)
	})

	t.Run("env replaces the JSON list rather than merging", func(t *testing.T) {
		cfg := &AppConfig{AdminIDs: []int64{1, 2}}
		require.NoError(t, cfg.applyAdminIDsEnv("42"))
		assert.Equal(t, []int64{42}, cfg.AdminIDs)
	})

	t.Run("malformed env fails loudly and leaves the list untouched", func(t *testing.T) {
		// Silently dropping the list would lock every admin out of /start_vote
		// with no clue why; refusing to start points at the typo instead.
		cfg := &AppConfig{AdminIDs: []int64{1}}
		assert.Error(t, cfg.applyAdminIDsEnv("42,nope"))
		assert.Equal(t, []int64{1}, cfg.AdminIDs)
	})
}

func TestAdminIDsFromJSON(t *testing.T) {
	cfg, err := parsreAppConfig(strings.NewReader(`{"admin_ids": [111, 222]}`))
	require.NoError(t, err)
	assert.Equal(t, []int64{111, 222}, cfg.AdminIDs)

	cfg, err = parsreAppConfig(strings.NewReader(`{}`))
	require.NoError(t, err)
	assert.Empty(t, cfg.AdminIDs)
}
