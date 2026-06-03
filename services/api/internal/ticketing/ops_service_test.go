package ticketing

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"event-ticket-system/internal/reservation"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapacityPressureAggregatesPublishedEvents(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	insertOpsEmployee(t, service, ctx, "E1001")
	insertOpsEmployee(t, service, ctx, "E1002")
	insertOpsEmployee(t, service, ctx, "E1003")
	fcfs := createPublishedEvent(t, service, ctx, engineeringEventRequest("FCFS pressure", 1, 1))
	lotteryReq := engineeringEventRequest("Lottery pressure", 2, 1)
	lotteryReq.AllocationMode = AllocationModeLottery
	lottery := createPublishedEvent(t, service, ctx, lotteryReq)
	_, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, fcfs.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "pressure-fcfs"})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1003", Role: RoleEmployee}, fcfs.EventID, BookingRequest{EmployeeID: "E1003", IdempotencyKey: "pressure-fcfs-waitlist"})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, lottery.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "pressure-lottery"})
	require.NoError(t, err)
	insertOpsRateLimitAudit(t, service, ctx, fcfs.EventID)

	pressure, err := service.CapacityPressure(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin})

	require.NoError(t, err)
	rows := capacityPressureByEvent(pressure)
	require.Contains(t, rows, fcfs.EventID)
	require.Contains(t, rows, lottery.EventID)
	assert.Equal(t, 1, rows[fcfs.EventID].ConfirmedCount)
	assert.Equal(t, 1, rows[fcfs.EventID].WaitlistCount)
	assert.Equal(t, 0, rows[fcfs.EventID].ReceivedCount)
	require.NotNil(t, rows[fcfs.EventID].RemainingCapacity)
	assert.Equal(t, 0, *rows[fcfs.EventID].RemainingCapacity)
	assert.Equal(t, reservation.PressureStateDisabled, rows[fcfs.EventID].ReservationState)
	assert.Equal(t, 0, rows[fcfs.EventID].ReservationCount)
	require.NotNil(t, rows[fcfs.EventID].RateLimitDropPerMin)
	assert.Equal(t, 1, *rows[fcfs.EventID].RateLimitDropPerMin)
	assert.Nil(t, rows[fcfs.EventID].RejectedPerMin)
	assert.Equal(t, 0, rows[lottery.EventID].ConfirmedCount)
	assert.Equal(t, 0, rows[lottery.EventID].WaitlistCount)
	assert.Equal(t, 1, rows[lottery.EventID].ReceivedCount)
	require.NotNil(t, rows[lottery.EventID].RemainingCapacity)
	assert.Equal(t, 2, *rows[lottery.EventID].RemainingCapacity)
	assert.Equal(t, 0, rows[lottery.EventID].ReservationCount)
	assert.Equal(t, reservation.PressureStateDisabled, rows[lottery.EventID].ReservationState)
	assert.Nil(t, rows[lottery.EventID].IdempotencyReplayPerMin)

	payload, err := json.Marshal(pressure)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "E1001")
	assert.NotContains(t, string(payload), "pressure-fcfs")
}

func TestCapacityPressureReturnsEmptyEvents(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	pressure, err := service.CapacityPressure(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin})

	require.NoError(t, err)
	assert.Empty(t, pressure.Events)
}

func TestCapacityPressureMarksReservationTelemetryUnavailable(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	service.WithReservationGate(&pressureGate{state: reservation.PressureStateAvailable, err: errors.New("redis unavailable")}, []byte("secret"))
	event := createPublishedEvent(t, service, ctx, engineeringEventRequest("Unavailable pressure", 1, 1))

	pressure, err := service.CapacityPressure(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin})

	require.NoError(t, err)
	rows := capacityPressureByEvent(pressure)
	require.Contains(t, rows, event.EventID)
	assert.Equal(t, reservation.PressureStateUnavailable, rows[event.EventID].ReservationState)
	assert.Equal(t, 0, rows[event.EventID].ReservationCount)
}

func TestCapacityPressureReadsActiveReservationTelemetry(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	gate := &pressureGate{state: reservation.PressureStateAvailable, activeCount: 3}
	service.WithReservationGate(gate, []byte("secret"))
	event := createPublishedEvent(t, service, ctx, engineeringEventRequest("Available pressure", 5, 1))
	otherEvent := createPublishedEvent(t, service, ctx, engineeringEventRequest("Available pressure 2", 5, 1))

	pressure, err := service.CapacityPressure(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin})

	require.NoError(t, err)
	rows := capacityPressureByEvent(pressure)
	require.Contains(t, rows, event.EventID)
	require.Contains(t, rows, otherEvent.EventID)
	assert.Equal(t, reservation.PressureStateAvailable, rows[event.EventID].ReservationState)
	assert.Equal(t, 3, rows[event.EventID].ReservationCount)
	assert.Equal(t, reservation.PressureStateAvailable, rows[otherEvent.EventID].ReservationState)
	assert.Equal(t, 3, rows[otherEvent.EventID].ReservationCount)
	assert.Equal(t, 1, gate.bulkCalls)
	assert.Equal(t, 0, gate.singleCalls)
}

func TestCapacityPressureRequiresOpsRole(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := service.CapacityPressure(ctx, Actor{ID: "E1001", Role: RoleEmployee})

	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))
	assertOpsAccessDeniedAudit(t, service, ctx, "capacity_pressure", "E1001", RoleEmployee)
}

func TestReportFreshnessReadsProjectionOffsets(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	service.WithClock(func() time.Time { return now })
	lastApplied := now.Add(-20 * time.Second)
	updatedAt := now.Add(-10 * time.Second)
	_, err := service.db.Exec(ctx, `UPDATE reporting_projection_offsets
		SET last_processed_at = $1, updated_at = $2
		WHERE projection_name = 'event_summary'`, lastApplied, updatedAt)
	require.NoError(t, err)

	freshness, err := service.ReportFreshness(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, 30)

	require.NoError(t, err)
	require.Len(t, freshness.Projections, 1)
	projection := freshness.Projections[0]
	assert.Equal(t, "event_summary", projection.Name)
	require.NotNil(t, projection.LastApplied)
	assert.True(t, lastApplied.Equal(*projection.LastApplied), "last_applied should match the stored instant")
	assert.Equal(t, 10, projection.LagSeconds)
	assert.False(t, projection.Degraded)
}

func TestReportFreshnessReturnsDegradedPlaceholderWhenOffsetMissing(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	_, err := service.db.Exec(ctx, `DELETE FROM reporting_projection_offsets WHERE projection_name = 'event_summary'`)
	require.NoError(t, err)

	freshness, err := service.ReportFreshness(ctx, Actor{ID: "system-1", Role: RoleSystemAdmin}, 30)

	require.NoError(t, err)
	require.Len(t, freshness.Projections, 1)
	assert.Equal(t, "event_summary", freshness.Projections[0].Name)
	assert.Nil(t, freshness.Projections[0].LastApplied)
	assert.True(t, freshness.Projections[0].Degraded)
}

func TestReportFreshnessTreatsSeededUnappliedOffsetAsDegraded(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	freshness, err := service.ReportFreshness(ctx, Actor{ID: "system-1", Role: RoleSystemAdmin}, 30)

	require.NoError(t, err)
	require.Len(t, freshness.Projections, 1)
	assert.Equal(t, "event_summary", freshness.Projections[0].Name)
	assert.Nil(t, freshness.Projections[0].LastApplied)
	assert.True(t, freshness.Projections[0].Degraded)
}

func TestReportFreshnessRejectsActivityAdmin(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := service.ReportFreshness(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, 30)

	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))
	assertOpsAccessDeniedAudit(t, service, ctx, "report_freshness", "admin-1", RoleActivityAdmin)
}

func TestOpsDashboardActivityAdminGetsReadSubset(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	dashboard, err := service.OpsDashboard(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, 30)

	require.NoError(t, err)
	assert.Empty(t, dashboard.Queues.Queues)
	assert.Empty(t, dashboard.ReportsFreshness.Projections)
	assert.Empty(t, dashboard.DeadLetterRecent)
	assert.Empty(t, dashboard.ReplayRecent)
	assert.NotNil(t, dashboard.CapacityPressure.Events)
}

func TestOpsDashboardHRAdminIncludesOpsFeeds(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	insertOpsReplayAudit(t, service, ctx)

	dashboard, err := service.OpsDashboard(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, 30)

	require.NoError(t, err)
	assert.Len(t, dashboard.Queues.Queues, 5)
	require.Len(t, dashboard.ReportsFreshness.Projections, 1)
	assert.Equal(t, "event_summary", dashboard.ReportsFreshness.Projections[0].Name)
	require.Len(t, dashboard.ReplayRecent, 1)
	replay := dashboard.ReplayRecent[0]
	assert.Equal(t, "aud_replay_recent", replay.AuditID)
	assert.Equal(t, RoleHRAdmin, replay.ActorRole)
	assert.Equal(t, outboxWorkerKindNotification, replay.Kind)
	assert.False(t, replay.DryRun)
	assert.Equal(t, 2, replay.AffectedCount)
	require.NotNil(t, replay.EnqueuedCount)
	assert.Equal(t, 2, *replay.EnqueuedCount)
	payload, err := json.Marshal(dashboard.ReplayRecent)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "replayed_rows")
	assert.NotContains(t, string(payload), "out_sensitive_replay_row")
}

func TestOpsDashboardSystemAdminIncludesOpsFeeds(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	dashboard, err := service.OpsDashboard(ctx, Actor{ID: "system-1", Role: RoleSystemAdmin}, 30)

	require.NoError(t, err)
	assert.Len(t, dashboard.Queues.Queues, 5)
	require.Len(t, dashboard.ReportsFreshness.Projections, 1)
	assert.Equal(t, "event_summary", dashboard.ReportsFreshness.Projections[0].Name)
}

func insertOpsEmployee(t *testing.T, service *Service, ctx context.Context, employeeID string) {
	t.Helper()
	_, err := service.db.Exec(ctx, `INSERT INTO employees
		(employee_id, full_name, department, site, job_grade, employment_status)
		VALUES ($1, $2, 'Engineering', 'Taipei HQ', 5, 'active')`, employeeID, employeeID+" User")
	require.NoError(t, err)
}

func insertOpsReplayAudit(t *testing.T, service *Service, ctx context.Context) {
	t.Helper()
	_, err := service.db.Exec(ctx, `INSERT INTO audit_logs
		(audit_id, actor_id, role, action, entity_type, entity_id, metadata, created_at)
		VALUES ($1, $2, $3, $4, 'outbox_queue', $5, $6::jsonb, $7)`,
		"aud_replay_recent",
		"hr-1",
		RoleHRAdmin,
		outboxReplayAction,
		outboxWorkerKindNotification,
		`{
			"kind": "notification",
			"dry_run": false,
			"affected_count": 2,
			"enqueued_count": 2,
			"replayed_rows": [{"outbox_id": "out_sensitive_replay_row"}]
		}`,
		time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)
}

func insertOpsRateLimitAudit(t *testing.T, service *Service, ctx context.Context, eventID string) {
	t.Helper()
	metadata, err := json.Marshal(map[string]string{"event_id": eventID, "scope": "event"})
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `INSERT INTO audit_logs
		(audit_id, actor_id, role, action, entity_type, entity_id, metadata, created_at)
		VALUES ($1, $2, $3, 'booking.rate_limited', 'event', $4, $5::jsonb, now())`,
		"aud_rate_limit_pressure",
		"system",
		RoleSystemAdmin,
		eventID,
		string(metadata),
	)
	require.NoError(t, err)
}

type pressureGate struct {
	state       string
	activeCount int
	err         error
	singleCalls int
	bulkCalls   int
}

func (g *pressureGate) Enabled() bool { return true }

func (g *pressureGate) Reserve(context.Context, string, string, string, reservation.CapacityProbe) (reservation.Hold, error) {
	return reservation.Hold{Outcome: reservation.OutcomeGranted}, nil
}

func (g *pressureGate) Confirm(context.Context, string, string) error { return nil }
func (g *pressureGate) Release(context.Context, string, string) error { return nil }

func (g *pressureGate) PressureSnapshot(context.Context, string) (reservation.PressureSnapshot, error) {
	g.singleCalls++
	if g.err != nil {
		return reservation.PressureSnapshot{}, g.err
	}
	return reservation.PressureSnapshot{State: g.state, ActiveCount: g.activeCount}, nil
}

func (g *pressureGate) PressureSnapshots(_ context.Context, eventIDs []string) (map[string]reservation.PressureSnapshot, error) {
	g.bulkCalls++
	if g.err != nil {
		return nil, g.err
	}
	snapshots := make(map[string]reservation.PressureSnapshot, len(eventIDs))
	for _, eventID := range eventIDs {
		snapshots[eventID] = reservation.PressureSnapshot{State: g.state, ActiveCount: g.activeCount}
	}
	return snapshots, nil
}

func capacityPressureByEvent(pressure CapacityPressure) map[string]CapacityPressureRow {
	rows := map[string]CapacityPressureRow{}
	for _, row := range pressure.Events {
		rows[row.EventID] = row
	}
	return rows
}

func assertOpsAccessDeniedAudit(t *testing.T, service *Service, ctx context.Context, resource string, actorID string, role string) {
	t.Helper()

	var storedActorID, storedRole string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT actor_id, role FROM audit_logs
		WHERE action = $1 AND entity_type = $2 AND entity_id = $3`,
		opsAccessDeniedAction, opsAccessDeniedEntityType, resource).Scan(&storedActorID, &storedRole))
	assert.Equal(t, actorID, storedActorID)
	assert.Equal(t, role, storedRole)

	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs
		WHERE action = $1 AND entity_type = $2 AND entity_id = $3`,
		opsAccessDeniedAction, opsAccessDeniedEntityType, resource)
	assert.Equal(t, resource, audit["resource"])
	assert.Equal(t, float64(403), audit["status"])
	assert.Equal(t, errRoleNotAllowed, audit["reason"])
}
