package ticketing

import (
	"context"
	"errors"
	"strings"

	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) Reports(ctx context.Context, actor Actor) (ReportsResult, error) {
	if err := requireAnyRole(actor, RoleActivityAdmin, RoleSystemAdmin, RoleHRAdmin); err != nil {
		return ReportsResult{}, err
	}
	rows, err := s.db.Query(ctx, `SELECT
			e.event_id, e.title, e.capacity_type, e.capacity, e.event_city, e.starts_at,
			COALESCE(reg.confirmed_count, 0) AS confirmed_count,
			COALESCE(reg.waitlist_count, 0) AS waitlist_count,
			COALESCE(reg.employee_count, 0) AS employee_count,
			COALESCE(reg.family_count, 0) AS family_count,
			COALESCE(tickets.ticket_count, 0) AS ticket_count,
			COALESCE(checkins.checkin_count, 0) AS checkin_count
		FROM events e
		LEFT JOIN (
			SELECT event_id,
				count(*) FILTER (WHERE status = 'confirmed') AS confirmed_count,
				count(*) FILTER (WHERE status = 'waitlisted') AS waitlist_count,
				count(DISTINCT employee_id) FILTER (WHERE status = 'confirmed') AS employee_count,
				COALESCE(sum(family_count) FILTER (WHERE status = 'confirmed'), 0) AS family_count
			FROM registrations
			GROUP BY event_id
		) reg ON reg.event_id = e.event_id
		LEFT JOIN (
			SELECT event_id, count(DISTINCT ticket_id) AS ticket_count
			FROM tickets
			GROUP BY event_id
		) tickets ON tickets.event_id = e.event_id
		LEFT JOIN (
			SELECT t.event_id, count(DISTINCT c.checkin_id) AS checkin_count
			FROM tickets t
			JOIN checkin_records c ON c.ticket_id = t.ticket_id
			GROUP BY t.event_id
		) checkins ON checkins.event_id = e.event_id
		ORDER BY e.starts_at DESC`)
	if err != nil {
		return ReportsResult{}, err
	}
	defer rows.Close()

	var reports []ReportRow
	for rows.Next() {
		var row ReportRow
		var capacity pgtype.Int4
		var eventCity string
		if err := rows.Scan(
			&row.EventID, &row.Title, &row.CapacityType, &capacity, &eventCity, &row.StartsAt,
			&row.ConfirmedCount, &row.WaitlistCount, &row.EmployeeCount, &row.FamilyCount,
			&row.TicketCount, &row.CheckinCount,
		); err != nil {
			return ReportsResult{}, err
		}
		row.TotalAttendeeCount = row.EmployeeCount + row.FamilyCount
		row.CityDistribution = map[string]int{reportCityKey(eventCity): row.TotalAttendeeCount}
		if capacity.Valid {
			value := int(capacity.Int32)
			row.Capacity = &value
			row.RemainingCapacity = intPtr(max(value-row.ConfirmedCount, 0))
		}
		reports = append(reports, row)
	}
	if err := rows.Err(); err != nil {
		return ReportsResult{}, err
	}

	var updatedAt *time.Time
	err = s.db.QueryRow(ctx, `SELECT updated_at FROM reporting_projection_offsets WHERE projection_name = 'event_summary'`).Scan(&updatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Warn("reporting projection offset lookup failed", "err", err)
	}

	return ReportsResult{
		Rows:                reports,
		ProjectionUpdatedAt: updatedAt,
	}, nil
}

func reportCityKey(city string) string {
	city = strings.TrimSpace(city)
	if city == "" {
		return "unknown"
	}
	return city
}
