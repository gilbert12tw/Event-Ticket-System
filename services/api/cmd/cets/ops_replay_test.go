package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunOpsReplayRejectsInvalidEventFiltersBeforeDatabaseConnect(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "unknown event type",
			args: []string{
				"ops", "replay",
				"--kind=notification",
				"--from=2026-05-28T10:00:00Z",
				"--to=2026-05-28T11:00:00Z",
				"--event-type=unknown.notification.event",
			},
			want: "replay event type is invalid",
		},
		{
			name: "event type wrong kind",
			args: []string{
				"ops", "replay",
				"--kind=notification",
				"--from=2026-05-28T10:00:00Z",
				"--to=2026-05-28T11:00:00Z",
				"--event-type=report.export.requested.v2",
			},
			want: "does not belong to worker kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "://invalid")

			err := run(tt.args, testLogger())

			require.ErrorContains(t, err, tt.want)
			require.NotContains(t, err.Error(), "invalid database URL")
		})
	}
}
