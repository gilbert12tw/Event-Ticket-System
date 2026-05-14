package ticketing

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) Reports(ctx context.Context, actor Actor) ([]ReportRow, error) {
	if err := requireRole(actor, RoleHRAdmin); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT
			e.event_id, e.title, e.capacity_type, e.capacity, e.starts_at,
			count(DISTINCT r.registration_id) FILTER (WHERE r.status = 'confirmed') AS confirmed_count,
			count(DISTINCT r.registration_id) FILTER (WHERE r.status = 'waitlisted') AS waitlist_count,
			count(DISTINCT t.ticket_id) AS ticket_count,
			count(DISTINCT c.checkin_id) AS checkin_count
		FROM events e
		LEFT JOIN registrations r ON r.event_id = e.event_id
		LEFT JOIN tickets t ON t.event_id = e.event_id
		LEFT JOIN checkin_records c ON c.ticket_id = t.ticket_id
		GROUP BY e.event_id, e.title, e.capacity_type, e.capacity, e.starts_at
		ORDER BY e.starts_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []ReportRow
	for rows.Next() {
		var row ReportRow
		var capacity pgtype.Int4
		if err := rows.Scan(&row.EventID, &row.Title, &row.CapacityType, &capacity, &row.StartsAt, &row.ConfirmedCount, &row.WaitlistCount, &row.TicketCount, &row.CheckinCount); err != nil {
			return nil, err
		}
		if capacity.Valid {
			value := int(capacity.Int32)
			row.Capacity = &value
			row.RemainingCapacity = intPtr(max(value-row.ConfirmedCount, 0))
		}
		reports = append(reports, row)
	}
	return reports, rows.Err()
}
