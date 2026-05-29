package ticketing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateReplayOutboxRequestNormalizesRegisteredEventFilters(t *testing.T) {
	base := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)

	req, err := ValidateReplayOutboxRequest(ReplayOutboxRequest{
		Kind: outboxWorkerKindNotification,
		From: base,
		To:   base.Add(time.Minute),
		EventTypes: []string{
			" booking.confirmed ",
			"notification.requested.v2",
			"booking.confirmed",
		},
		DryRun: true,
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"booking.confirmed", "notification.requested.v2"}, req.EventTypes)
}

func TestValidateReplayOutboxRequestRejectsUnknownEventFilter(t *testing.T) {
	base := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)

	_, err := ValidateReplayOutboxRequest(ReplayOutboxRequest{
		Kind:       outboxWorkerKindNotification,
		From:       base,
		To:         base.Add(time.Minute),
		EventTypes: []string{"unknown.notification.event"},
		DryRun:     true,
	})

	require.Error(t, err)
	assert.Equal(t, 400, ErrorStatus(err))
	assert.Contains(t, err.Error(), "replay event type is invalid")
}
