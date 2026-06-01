package ticketing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutboxV2MetadataFollowsContractRecipes(t *testing.T) {
	ts := "2026-05-19T12:34:56Z"
	cases := []struct {
		name               string
		eventType          string
		payload            map[string]interface{}
		wantIdempotencyKey string
		wantPartitionKey   string
	}{
		{
			name:      "registration confirmed",
			eventType: "registration.confirmed.v2",
			payload: map[string]interface{}{
				"registration_id": "reg_1",
				"event_id":        "evt_1",
			},
			wantIdempotencyKey: "registration.confirmed:reg_1",
			wantPartitionKey:   "evt_1",
		},
		{
			name:      "registration cancelled",
			eventType: "registration.cancelled.v2",
			payload: map[string]interface{}{
				"registration_id": "reg_1",
				"cancelled_at":    ts,
				"event_id":        "evt_1",
			},
			wantIdempotencyKey: "registration.cancelled:reg_1:" + ts,
			wantPartitionKey:   "evt_1",
		},
		{
			name:      "registration waitlisted",
			eventType: "registration.waitlisted.v2",
			payload: map[string]interface{}{
				"registration_id": "reg_1",
				"event_id":        "evt_1",
			},
			wantIdempotencyKey: "registration.waitlisted:reg_1",
			wantPartitionKey:   "evt_1",
		},
		{
			name:      "registration received",
			eventType: "registration.received.v2",
			payload: map[string]interface{}{
				"registration_id": "reg_1",
				"event_id":        "evt_1",
			},
			wantIdempotencyKey: "registration.received:reg_1",
			wantPartitionKey:   "evt_1",
		},
		{
			name:      "registration promoted",
			eventType: "registration.promoted.v2",
			payload: map[string]interface{}{
				"registration_id": "reg_1",
				"promoted_at":     ts,
				"event_id":        "evt_1",
			},
			wantIdempotencyKey: "registration.promoted:reg_1:" + ts,
			wantPartitionKey:   "evt_1",
		},
		{
			name:      "ticket issued",
			eventType: "ticket.issued.v2",
			payload: map[string]interface{}{
				"ticket_id": "tkt_1",
				"event_id":  "evt_1",
			},
			wantIdempotencyKey: "ticket.issued:tkt_1",
			wantPartitionKey:   "evt_1",
		},
		{
			name:      "ticket revoked",
			eventType: "ticket.revoked.v2",
			payload: map[string]interface{}{
				"ticket_id": "tkt_1",
				"event_id":  "evt_1",
			},
			wantIdempotencyKey: "ticket.revoked:tkt_1",
			wantPartitionKey:   "evt_1",
		},
		{
			name:      "ticket expired",
			eventType: "ticket.expired.v2",
			payload: map[string]interface{}{
				"ticket_id": "tkt_1",
				"event_id":  "evt_1",
			},
			wantIdempotencyKey: "ticket.expired:tkt_1",
			wantPartitionKey:   "evt_1",
		},
		{
			name:      "checkin recorded",
			eventType: "checkin.recorded.v2",
			payload: map[string]interface{}{
				"ticket_id": "tkt_1",
				"event_id":  "evt_1",
			},
			wantIdempotencyKey: "checkin.recorded:tkt_1",
			wantPartitionKey:   "evt_1",
		},
		{
			name:      "notification requested",
			eventType: "notification.requested.v2",
			payload: map[string]interface{}{
				"notification_id":       "ntf_1",
				"recipient_employee_id": "EMP-1",
			},
			wantIdempotencyKey: "notification.requested:ntf_1",
			wantPartitionKey:   "EMP-1",
		},
		{
			name:      "reservation compensation",
			eventType: "reservation.compensation.release_required.v2",
			payload: map[string]interface{}{
				"reservation_id": "resv_1",
				"event_id":       "evt_1",
			},
			wantIdempotencyKey: "reservation.compensation.release_required:resv_1",
			wantPartitionKey:   "evt_1",
		},
		{
			name:      "report export requested",
			eventType: outboxEventReportExportRequestedV2,
			payload: map[string]interface{}{
				"export_id": "exp_1",
			},
			wantIdempotencyKey: "report.export.requested:exp_1",
			wantPartitionKey:   "exp_1",
		},
		{
			name:      "report export completed",
			eventType: "report.export.completed.v2",
			payload: map[string]interface{}{
				"export_id": "exp_1",
			},
			wantIdempotencyKey: "report.export.completed:exp_1",
			wantPartitionKey:   "exp_1",
		},
		{
			name:      "report export failed",
			eventType: "report.export.failed.v2",
			payload: map[string]interface{}{
				"export_id": "exp_1",
			},
			wantIdempotencyKey: "report.export.failed:exp_1",
			wantPartitionKey:   "exp_1",
		},
		{
			name:      "hr sync batch completed",
			eventType: "hr_sync.batch.completed.v2",
			payload: map[string]interface{}{
				"batch_id": "hrb_1",
			},
			wantIdempotencyKey: "hr_sync.batch.completed:hrb_1",
			wantPartitionKey:   "hrb_1",
		},
		{
			name:      "eligibility impact review",
			eventType: "eligibility.impact_review.created.v2",
			payload: map[string]interface{}{
				"review_id": "rev_1",
				"event_id":  "evt_1",
			},
			wantIdempotencyKey: "eligibility.impact_review.created:rev_1",
			wantPartitionKey:   "evt_1",
		},
		{
			name:      "reporting projection update",
			eventType: outboxEventReportingProjectionUpdateRequiredV2,
			payload: map[string]interface{}{
				"projection_name":  "event_participation_summary",
				"aggregate_id":     "evt_1",
				"trigger_event_id": "out_1",
			},
			wantIdempotencyKey: "reporting.projection.update_required:event_participation_summary:evt_1:out_1",
			wantPartitionKey:   "event_participation_summary",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idempotencyKey, partitionKey, err := outboxV2Metadata(tc.eventType, "aggregate-fallback", "out_1", tc.payload)

			require.NoError(t, err)
			assert.Equal(t, tc.wantIdempotencyKey, idempotencyKey)
			assert.Equal(t, tc.wantPartitionKey, partitionKey)
		})
	}
}

func TestOutboxV2MetadataPreservesLegacyFallback(t *testing.T) {
	idempotencyKey, partitionKey, err := outboxV2Metadata("booking.confirmed", "reg_1", "out_1", nil)

	require.NoError(t, err)
	assert.Equal(t, "booking.confirmed:out_1", idempotencyKey)
	assert.Equal(t, "reg_1", partitionKey)
}

func TestOutboxV2MetadataRequiresRecipeFields(t *testing.T) {
	_, _, err := outboxV2Metadata("notification.requested.v2", "ntf_1", "out_1", map[string]interface{}{
		"recipient_employee_id": "EMP-1",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "payload.notification_id")
}
