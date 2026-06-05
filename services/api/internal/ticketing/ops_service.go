package ticketing

import (
	"context"
	"database/sql"
	"time"

	"event-ticket-system/internal/reservation"
)

const (
	defaultOpsLimit                    = 50
	defaultFreshnessThresholdSeconds   = 60
	defaultRecentDeadLetterResultLimit = 5
)

func (s *Service) CapacityPressure(ctx context.Context, actor Actor) (CapacityPressure, error) {
	if err := s.requireOpsAnyRole(ctx, actor, "capacity_pressure", RoleActivityAdmin, RoleHRAdmin, RoleSystemAdmin); err != nil {
		return CapacityPressure{}, err
	}
	rows, err := s.db.Query(ctx, `WITH registration_counts AS (
			SELECT event_id,
				count(*) FILTER (WHERE status = 'confirmed')::int AS confirmed_count,
				count(*) FILTER (WHERE status = 'waitlisted')::int AS waitlist_count,
				count(*) FILTER (WHERE status = 'received')::int AS received_count
			FROM registrations
			GROUP BY event_id
		), rate_limit_drops AS (
			SELECT metadata->>'event_id' AS event_id, count(*)::int AS drops
			FROM audit_logs
			WHERE action = 'booking.rate_limited'
				AND created_at >= now() - interval '1 minute'
				AND metadata ? 'event_id'
			GROUP BY metadata->>'event_id'
		)
		SELECT e.event_id, e.capacity_type,
			COALESCE(rc.confirmed_count, 0)::int,
			COALESCE(rc.waitlist_count, 0)::int,
			COALESCE(rc.received_count, 0)::int,
			CASE
				WHEN e.capacity_type = 'limited' AND e.capacity IS NOT NULL
					THEN GREATEST(e.capacity - COALESCE(rc.confirmed_count, 0), 0)
				ELSE NULL
			END AS remaining_capacity,
			COALESCE(rld.drops, 0)::int
		FROM events e
		LEFT JOIN registration_counts rc ON rc.event_id = e.event_id
		LEFT JOIN rate_limit_drops rld ON rld.event_id = e.event_id
		WHERE e.archived_at IS NULL
			AND e.status IN ('published', 'closed')
		ORDER BY e.registration_close ASC, e.starts_at ASC, e.event_id ASC
		LIMIT $1`, defaultOpsLimit)
	if err != nil {
		return CapacityPressure{}, err
	}
	defer rows.Close()

	pressure := CapacityPressure{Events: []CapacityPressureRow{}}
	for rows.Next() {
		row, err := scanCapacityPressureRow(rows)
		if err != nil {
			return CapacityPressure{}, err
		}
		pressure.Events = append(pressure.Events, row)
	}
	if err := rows.Err(); err != nil {
		return CapacityPressure{}, err
	}
	s.applyReservationPressure(ctx, pressure.Events)
	return pressure, nil
}

func (s *Service) applyReservationPressure(ctx context.Context, rows []CapacityPressureRow) {
	if len(rows) == 0 {
		return
	}
	reader, ok := s.reservationGate.(reservation.PressureReader)
	if !ok {
		state := reservation.PressureStateDisabled
		if s.reservationGate.Enabled() {
			state = reservation.PressureStateUnavailable
		}
		applyReservationState(rows, state)
		return
	}
	eventIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		eventIDs = append(eventIDs, row.EventID)
	}
	snapshots, err := reader.PressureSnapshots(ctx, eventIDs)
	if err != nil {
		applyReservationState(rows, reservation.PressureStateUnavailable)
		return
	}
	for i := range rows {
		snapshot, ok := snapshots[rows[i].EventID]
		if !ok {
			rows[i].ReservationState = reservation.PressureStateUnavailable
			continue
		}
		rows[i].ReservationState = snapshot.State
		rows[i].ReservationCount = snapshot.ActiveCount
	}
}

func applyReservationState(rows []CapacityPressureRow, state string) {
	for i := range rows {
		rows[i].ReservationState = state
	}
}

func (s *Service) ReportFreshness(ctx context.Context, actor Actor, thresholdSeconds int) (ReportFreshness, error) {
	if err := s.requireOpsFeedRole(ctx, actor, "report_freshness"); err != nil {
		return ReportFreshness{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT projection_name,
			NULLIF(last_processed_at, '-infinity'::timestamptz),
			updated_at
		FROM reporting_projection_offsets
		ORDER BY projection_name ASC`)
	if err != nil {
		return ReportFreshness{}, err
	}
	defer rows.Close()

	freshness := ReportFreshness{Projections: []ReportFreshnessProjection{}}
	now := s.now()
	for rows.Next() {
		var name string
		var lastApplied sql.NullTime
		var updatedAt sql.NullTime
		if err := rows.Scan(&name, &lastApplied, &updatedAt); err != nil {
			return ReportFreshness{}, err
		}
		freshness.Projections = append(freshness.Projections, buildReportFreshnessProjection(
			name,
			nullableTimePtr(lastApplied),
			nullableTimePtr(updatedAt),
			thresholdSeconds,
			now,
		))
	}
	if err := rows.Err(); err != nil {
		return ReportFreshness{}, err
	}
	if len(freshness.Projections) == 0 {
		freshness.Projections = append(freshness.Projections, buildReportFreshnessProjection(
			"event_summary",
			nil,
			nil,
			thresholdSeconds,
			now,
		))
	}
	return freshness, nil
}

func (s *Service) OpsDashboard(ctx context.Context, actor Actor, thresholdSeconds int) (OpsDashboard, error) {
	if err := s.requireOpsAnyRole(ctx, actor, "dashboard", RoleActivityAdmin, RoleHRAdmin, RoleSystemAdmin); err != nil {
		return OpsDashboard{}, err
	}
	pressure, err := s.CapacityPressure(ctx, actor)
	if err != nil {
		return OpsDashboard{}, err
	}
	dashboard := OpsDashboard{
		CapacityPressure: pressure,
		Queues:           OutboxQueueStatus{Queues: []OutboxQueueStatusRow{}},
		ReportsFreshness: ReportFreshness{Projections: []ReportFreshnessProjection{}},
	}
	if actor.Role == RoleActivityAdmin {
		return dashboard, nil
	}
	queues, err := s.OutboxQueueStatus(ctx, actor)
	if err != nil {
		return OpsDashboard{}, err
	}
	freshness, err := s.ReportFreshness(ctx, actor, thresholdSeconds)
	if err != nil {
		return OpsDashboard{}, err
	}
	deliveries, err := s.NotificationDeliveryOpsFeed(ctx, actor, NotificationDeliveryOpsQuery{
		Status: deliveryStatusDeadLetter,
		Limit:  defaultRecentDeadLetterResultLimit,
	})
	if err != nil {
		return OpsDashboard{}, err
	}
	replays, err := s.recentOutboxReplays(ctx)
	if err != nil {
		return OpsDashboard{}, err
	}
	dashboard.Queues = queues
	dashboard.ReportsFreshness = freshness
	dashboard.DeadLetterRecent = deliveries.Deliveries
	dashboard.ReplayRecent = replays
	return dashboard, nil
}

type capacityPressureScanner interface {
	Scan(dest ...interface{}) error
}

func scanCapacityPressureRow(row capacityPressureScanner) (CapacityPressureRow, error) {
	var pressure CapacityPressureRow
	var remaining sql.NullInt64
	var rateLimitDrops int
	if err := row.Scan(
		&pressure.EventID,
		&pressure.CapacityType,
		&pressure.ConfirmedCount,
		&pressure.WaitlistCount,
		&pressure.ReceivedCount,
		&remaining,
		&rateLimitDrops,
	); err != nil {
		return CapacityPressureRow{}, err
	}
	if remaining.Valid {
		value := int(remaining.Int64)
		pressure.RemainingCapacity = &value
	}
	pressure.RateLimitDropPerMin = &rateLimitDrops
	return pressure, nil
}

func buildReportFreshnessProjection(name string, lastApplied, updatedAt *time.Time, thresholdSeconds int, now time.Time) ReportFreshnessProjection {
	if thresholdSeconds <= 0 {
		thresholdSeconds = defaultFreshnessThresholdSeconds
	}
	projection := ReportFreshnessProjection{
		Name:        name,
		LastApplied: lastApplied,
		Degraded:    true,
	}
	if updatedAt == nil {
		return projection
	}
	lag := int(now.Sub(*updatedAt).Seconds())
	if lag < 0 {
		lag = 0
	}
	projection.LagSeconds = lag
	projection.Degraded = lastApplied == nil || lag > thresholdSeconds
	return projection
}

func nullableTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	at := value.Time
	return &at
}
