package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunOpsRejectsUnknownCommandWithoutEcho(t *testing.T) {
	err := run([]string{"ops", "e1001@cets.local"}, testLogger())

	require.ErrorContains(t, err, "unknown ops command")
	require.NotContains(t, err.Error(), "e1001@cets.local")
}

func TestRunOpsOutboxStatsRejectsUnknownArgWithoutEcho(t *testing.T) {
	t.Setenv("DATABASE_URL", "://invalid")

	err := run([]string{"ops", "outbox-stats", "--employee=e1001@cets.local"}, testLogger())

	require.ErrorContains(t, err, "unknown ops outbox-stats argument")
	require.NotContains(t, err.Error(), "e1001@cets.local")
	require.NotContains(t, err.Error(), "invalid database URL")
}

func TestRunOpsReplayRejectsUnknownArgWithoutEcho(t *testing.T) {
	t.Setenv("DATABASE_URL", "://invalid")

	err := run([]string{"ops", "replay", "--employee=e1001@cets.local"}, testLogger())

	require.ErrorContains(t, err, "unknown ops replay argument")
	require.NotContains(t, err.Error(), "e1001@cets.local")
	require.NotContains(t, err.Error(), "invalid database URL")
}
