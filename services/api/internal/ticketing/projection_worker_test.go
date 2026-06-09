package ticketing

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"event-ticket-system/internal/observability"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectionWorkerOptions returns OutboxProcessorOptions for the projection
// worker kind. No sender or report store is required.
func projectionWorkerOptions() OutboxProcessorOptions {
	return OutboxProcessorOptions{
		WorkerKinds: []string{outboxWorkerKindProjection},
		MaxAttempts: 3,
	}
}

// insertProjectionOutbox seeds a reporting.projection.update_required.v2
// outbox event in the schema_version=1 flat payload format.
// outboxID is the PK; eventID is the aggregate (domain event); innerType is
// the trigger (e.g. "booking.confirmed"); department may be empty.
func insertProjectionOutbox(
	t *testing.T,
	service *Service,
	ctx context.Context,
	outboxID string,
	eventID string,
	innerType string,
	department string,
) {
	t.Helper()
	payload := map[string]interface{}{
		"aggregate_id":     eventID,
		"inner_event_type": innerType,
		"department":       department,
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `
		INSERT INTO outbox_events
			(outbox_id, aggregate_id, event_type, payload, publish_status, schema_version, attempts, available_at)
		VALUES ($1, $2, $3, $4::jsonb, 'pending', 1, 0, now() - interval '1 second')`,
		outboxID,
		outboxID+"-agg",
		outboxEventReportingProjectionUpdateRequiredV2,
		string(payloadJSON),
	)
	require.NoError(t, err)
}

// insertProjectionOutboxAt seeds a projection outbox event with a specific
// created_at timestamp (for lag-metric tests).
func insertProjectionOutboxAt(
	t *testing.T,
	service *Service,
	ctx context.Context,
	outboxID string,
	eventID string,
	innerType string,
	createdAt time.Time,
) {
	t.Helper()
	payload := map[string]interface{}{
		"aggregate_id":     eventID,
		"inner_event_type": innerType,
	}
	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `
		INSERT INTO outbox_events
			(outbox_id, aggregate_id, event_type, payload, publish_status, schema_version, attempts, available_at, created_at)
		VALUES ($1, $2, $3, $4::jsonb, 'pending', 1, 0, $5, $5)`,
		outboxID,
		outboxID+"-agg",
		outboxEventReportingProjectionUpdateRequiredV2,
		string(payloadJSON),
		createdAt,
	)
	require.NoError(t, err)
}

// insertProjectionOutboxV2 seeds a schema_version=2 projection envelope whose
// inner type is resolved by the worker via an outbox_events lookup on
// trigger_event_id. The trigger row (event_type=innerType, already published so
// the projection worker never claims it) is seeded only when innerType != "" —
// pass innerType="" to simulate a GC'd / missing trigger row.
func insertProjectionOutboxV2(
	t *testing.T,
	service *Service,
	ctx context.Context,
	outboxID string,
	eventID string,
	triggerID string,
	innerType string,
	department string,
) {
	t.Helper()
	if innerType != "" {
		_, err := service.db.Exec(ctx, `
			INSERT INTO outbox_events
				(outbox_id, aggregate_id, event_type, payload, publish_status, schema_version, attempts, available_at)
			VALUES ($1, $2, $3, '{}'::jsonb, 'published', 1, 0, now() - interval '2 seconds')`,
			triggerID, eventID, innerType)
		require.NoError(t, err)
	}
	const projectionName = projectionProjectionName
	idempotencyKey := "reporting.projection.update_required:" + projectionName + ":" + eventID + ":" + triggerID
	envelope := map[string]interface{}{
		"event_id":        outboxID,
		"event_type":      outboxEventReportingProjectionUpdateRequiredV2,
		"schema_version":  2,
		"occurred_at":     "2026-05-28T10:00:00Z",
		"idempotency_key": idempotencyKey,
		"partition_key":   projectionName,
		"payload": map[string]interface{}{
			"projection_name":  projectionName,
			"aggregate_id":     eventID,
			"trigger_event_id": triggerID,
			"department":       department,
		},
	}
	body, err := json.Marshal(envelope)
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `
		INSERT INTO outbox_events
			(outbox_id, aggregate_id, event_type, payload, publish_status, schema_version, attempts,
			 available_at, idempotency_key, partition_key)
		VALUES ($1, $2, $3, $4::jsonb, 'pending', 2, 0, now() - interval '1 second', $5, $6)`,
		outboxID, eventID, outboxEventReportingProjectionUpdateRequiredV2, string(body), idempotencyKey, projectionName)
	require.NoError(t, err)
}

// readEventSummary reads the current reporting_event_summary row.
func readEventSummary(t *testing.T, service *Service, ctx context.Context, eventID string) eventSummaryRow {
	t.Helper()
	row, err := getEventSummaryRow(ctx, service.db, eventID)
	require.NoError(t, err)
	return row
}

// readProjectionOffset reads the last_processed_outbox_id for the named
// projection, or "" when no row exists.
func readProjectionOffset(t *testing.T, service *Service, ctx context.Context, name string) string {
	t.Helper()
	offset, err := getProjectionOffset(ctx, service.db, name)
	if err != nil {
		return ""
	}
	return offset
}

// runProjectionWorkerOnce runs exactly one projection outbox event through the
// worker and returns (processed, err).
func runProjectionWorkerOnce(service *Service, ctx context.Context) (int, error) {
	return service.ProcessOutboxOnceWithOptions(ctx, projectionWorkerOptions())
}

// ---- Test cases (10 total) ---- //

// AC-1 / Test 1: a booking.confirmed event increments confirmed_count and
// updates department_breakdown.
func TestProjectionWorker_ConfirmedBookingUpdatesCount(t *testing.T) {
	service, ctx := newWorkerTest(t)
	insertProjectionOutbox(t, service, ctx, "ob-1", "evt_1", projectionInnerTypeBookingConfirmed, "Engineering")

	processed, err := runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	row := readEventSummary(t, service, ctx, "evt_1")
	assert.Equal(t, 1, row.ConfirmedCount)
	assert.Equal(t, 1, row.DepartmentBreakdown["Engineering"])
}

// AC-3 / Test 2: two confirmed then one cancelled leaves confirmed_count=1 and
// cancelled_count=1.
func TestProjectionWorker_CancelledBookingDecrementsCount(t *testing.T) {
	service, ctx := newWorkerTest(t)
	insertProjectionOutbox(t, service, ctx, "ob-1", "evt_1", projectionInnerTypeBookingConfirmed, "Engineering")
	insertProjectionOutbox(t, service, ctx, "ob-2", "evt_1", projectionInnerTypeBookingConfirmed, "Engineering")
	insertProjectionOutbox(t, service, ctx, "ob-3", "evt_1", projectionInnerTypeBookingCancelled, "Engineering")

	for i := 0; i < 3; i++ {
		_, err := runProjectionWorkerOnce(service, ctx)
		require.NoError(t, err)
	}

	row := readEventSummary(t, service, ctx, "evt_1")
	assert.Equal(t, 1, row.ConfirmedCount)
	assert.Equal(t, 1, row.CancelledCount)
}

// Test 3: counts never go below zero even if a cancel arrives with no prior
// confirmed.
func TestProjectionWorker_CountNeverGoesBelowZero(t *testing.T) {
	service, ctx := newWorkerTest(t)
	insertProjectionOutbox(t, service, ctx, "ob-1", "evt_1", projectionInnerTypeBookingCancelled, "Engineering")

	_, err := runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)

	row := readEventSummary(t, service, ctx, "evt_1")
	assert.Equal(t, 0, row.ConfirmedCount, "confirmed_count must not go below 0")
}

// AC-2 / Test 4: replaying the same outbox event twice must NOT double-count.
func TestProjectionWorker_IdempotentReplay(t *testing.T) {
	service, ctx := newWorkerTest(t)
	// Seed the primary event with a higher outbox ID.
	now := time.Now().UTC().Truncate(time.Microsecond).Add(-10 * time.Second)
	insertProjectionOutboxAt(t, service, ctx, "ob-500", "evt_1", projectionInnerTypeBookingConfirmed, now)

	// Process first time.
	processed, err := runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	// Seed a second outbox row with a LOWER outboxID to simulate stale replay.
	// We give it an older timestamp to ensure it's treated as older.
	insertProjectionOutboxAt(t, service, ctx, "ob-300", "evt_1", projectionInnerTypeBookingConfirmed, now.Add(-1*time.Minute))
	_, err = runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)

	row := readEventSummary(t, service, ctx, "evt_1")
	assert.Equal(t, 1, row.ConfirmedCount, "replayed stale event must not increment count again")
}

// Test 5: an older offset must not overwrite a newer projection state.
func TestProjectionWorker_OlderEventDoesNotOverwriteNewer(t *testing.T) {
	service, ctx := newWorkerTest(t)

	now := time.Now().UTC().Truncate(time.Microsecond).Add(-10 * time.Second)

	// Process ob-010 first — sets confirmed_count to 1.
	insertProjectionOutboxAt(t, service, ctx, "ob-010", "evt_1", projectionInnerTypeBookingConfirmed, now)
	_, err := runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)

	// Then replay ob-003 with an OLDER created_at.
	// The ON CONFLICT guard prevents it from overwriting the state written by ob-010.
	insertProjectionOutboxAt(t, service, ctx, "ob-003", "evt_1", projectionInnerTypeBookingCancelled, now.Add(-1*time.Minute))
	_, err = runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)

	row := readEventSummary(t, service, ctx, "evt_1")
	assert.Equal(t, 1, row.ConfirmedCount, "older event must not overwrite confirmed_count set by newer event")

	expectedOffset := projectionOffsetKey(now, "ob-010")
	assert.Equal(t, expectedOffset, row.LastEventOffset, "last_event_offset must remain at newer offset")
}

// AC-4 / Test 6: last_processed_outbox_id advances after every successful event.
func TestProjectionWorker_OffsetAdvancesAfterProcessing(t *testing.T) {
	service, ctx := newWorkerTest(t)
	insertProjectionOutbox(t, service, ctx, "ob-1", "evt_1", projectionInnerTypeBookingConfirmed, "")
	insertProjectionOutbox(t, service, ctx, "ob-2", "evt_1", projectionInnerTypeBookingConfirmed, "")
	insertProjectionOutbox(t, service, ctx, "ob-3", "evt_1", projectionInnerTypeBookingConfirmed, "")

	for i := 0; i < 3; i++ {
		_, err := runProjectionWorkerOnce(service, ctx)
		require.NoError(t, err)
	}

	offset := readProjectionOffset(t, service, ctx, projectionProjectionName)
	// Must be the composite key (20-digit unix-nano | outbox id), not a raw id —
	// guards against a regression that reverts to writing claim.outboxID.
	assert.Regexp(t, `^\d{20}\|ob-3$`, offset)
}

// schema_version=2 envelope: inner type is resolved from outbox_events via
// trigger_event_id, then the aggregate is updated like a v1 event.
func TestProjectionWorker_V2EnvelopeResolvesTriggerType(t *testing.T) {
	service, ctx := newWorkerTest(t)
	insertProjectionOutboxV2(t, service, ctx, "ob-v2-1", "evt_1", "trig-1", projectionInnerTypeBookingConfirmed, "Engineering")

	processed, err := runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	row := readEventSummary(t, service, ctx, "evt_1")
	assert.Equal(t, 1, row.ConfirmedCount)
	assert.Equal(t, 1, row.DepartmentBreakdown["Engineering"])
}

// schema_version=2 envelope whose trigger row is missing (published/GC'd before
// the projection event drains) is skipped gracefully — no error, no aggregate
// change — and still consumed (marked published) so it does not loop.
func TestProjectionWorker_V2EnvelopeTriggerGCSkipped(t *testing.T) {
	service, ctx := newWorkerTest(t)
	insertProjectionOutboxV2(t, service, ctx, "ob-v2-gc", "evt_1", "trig-missing", "", "")

	processed, err := runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	var summaryRows int
	require.NoError(t, service.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM reporting_event_summary WHERE event_id = $1`, "evt_1").Scan(&summaryRows))
	assert.Equal(t, 0, summaryRows, "GC'd-trigger event must not create an aggregate row")

	var pending int
	require.NoError(t, service.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM outbox_events WHERE outbox_id = $1 AND publish_status = 'pending'`, "ob-v2-gc").Scan(&pending))
	assert.Equal(t, 0, pending, "skipped projection event must be marked published, not left pending")
}

// AC-7 / Test 7: unknown inner event types are skipped without error or
// dead-letter; confirmed_count remains unchanged.
func TestProjectionWorker_UnknownInnerEventTypeIsSkipped(t *testing.T) {
	service, ctx := newWorkerTest(t)
	insertProjectionOutbox(t, service, ctx, "ob-1", "evt_1", "booking.unknown_v99", "")

	processed, err := runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	var deadLetterCount int
	require.NoError(t, service.db.QueryRow(ctx,
		`SELECT count(*) FROM outbox_events WHERE outbox_id = 'ob-1' AND publish_status = 'dead_letter'`,
	).Scan(&deadLetterCount))
	assert.Equal(t, 0, deadLetterCount, "unknown inner types must not be dead-lettered")

	row := readEventSummary(t, service, ctx, "evt_1")
	assert.Equal(t, 0, row.ConfirmedCount, "confirmed_count must be unchanged for unknown inner type")
}

// AC-8 / Test 8: crash recovery — worker processes from last committed offset
// with no double-counts.
func TestProjectionWorker_CrashRecovery(t *testing.T) {
	service, ctx := newWorkerTest(t)
	baseTime := time.Now().UTC()
	insertProjectionOutboxAt(t, service, ctx, "ob-1", "evt_1", projectionInnerTypeBookingConfirmed, baseTime)
	insertProjectionOutboxAt(t, service, ctx, "ob-2", "evt_1", projectionInnerTypeBookingConfirmed, baseTime.Add(1*time.Second))

	// Process ob-1 and ob-2 normally.
	_, err := runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)
	_, err = runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)

	// Simulate a "restart": ob-1 is re-queued as ob-1b with the same inner
	// event but a lower offset than the current watermark — idempotency guard
	// must prevent double-count.
	insertProjectionOutboxAt(t, service, ctx, "ob-1b", "evt_1", projectionInnerTypeBookingConfirmed, baseTime)
	_, err = runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)

	row := readEventSummary(t, service, ctx, "evt_1")
	assert.Equal(t, 2, row.ConfirmedCount, "no double-count after crash recovery replay")
}

// AC-5 / Test 9: lag metric is recorded after processing.
func TestProjectionWorker_LagMetricRecorded(t *testing.T) {
	service, ctx := newWorkerTest(t)
	// Seed an event with created_at = now() - 45s.
	past := time.Now().Add(-45 * time.Second)
	insertProjectionOutboxAt(t, service, ctx, "ob-lag", "evt_lag", projectionInnerTypeBookingConfirmed, past)

	// Reset global metrics so the snapshot is clean.
	service.metrics = observability.NewRegistry()

	_, err := runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)

	snap := service.metrics.ProjectionSnapshot()
	assert.GreaterOrEqual(t, snap.LagCount, uint64(1), "at least one lag observation expected")
	// The lag should be positive (the lease is acquired after created_at).
	assert.Greater(t, snap.LagSum, 0.0, "lag sum must be positive")
}

// AC-7 (AC-10) / Test 10: projection worker does not process notification
// events — an event_type of 'booking.confirmed' (notification kind) must not
// be claimed by the projection worker.
func TestProjectionWorker_DoesNotProcessNotificationEvents(t *testing.T) {
	service, ctx := newWorkerTest(t)
	// Insert a notification-kind outbox event (not a projection event).
	_, err := service.db.Exec(ctx, `
		INSERT INTO outbox_events
			(outbox_id, aggregate_id, event_type, payload, publish_status, schema_version, attempts, available_at)
		VALUES ($1, $2, 'booking.confirmed', '{"employee_id":"E1001"}'::jsonb, 'pending', 1, 0, now() - interval '1 second')`,
		fmt.Sprintf("ob-notif-%d", time.Now().UnixNano()),
		"notif-agg",
	)
	require.NoError(t, err)

	// The projection worker should not pick up this event.
	processed, err := runProjectionWorkerOnce(service, ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, processed, "projection worker must not claim notification-kind events")

	// reporting_event_summary must remain empty.
	var count int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM reporting_event_summary`).Scan(&count))
	assert.Equal(t, 0, count)
}
