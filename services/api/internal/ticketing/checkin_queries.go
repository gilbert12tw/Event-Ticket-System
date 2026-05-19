package ticketing

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Service) findCheckinByTicketTx(ctx context.Context, tx pgx.Tx, ticketID string) (CheckinResponse, bool, error) {
	var response CheckinResponse
	err := tx.QueryRow(ctx, `SELECT c.checkin_id, c.ticket_id, t.event_id, ev.title, t.employee_id, c.status, c.scanned_at, c.staff_id,
			r.family_count, e.full_name, e.department, e.site
		FROM checkin_records c
		JOIN tickets t ON t.ticket_id = c.ticket_id
		JOIN events ev ON ev.event_id = t.event_id
		JOIN registrations r ON r.registration_id = t.registration_id
		JOIN employees e ON e.employee_id = t.employee_id
		WHERE c.ticket_id = $1`, ticketID).
		Scan(&response.CheckinID, &response.TicketID, &response.EventID, &response.EventTitle, &response.EmployeeID, &response.Status, &response.ScannedAt, &response.FirstScannedBy,
			&response.FamilyCount, &response.Holder.DisplayName, &response.Holder.Department, &response.Holder.City)
	if errors.Is(err, pgx.ErrNoRows) {
		return CheckinResponse{}, false, nil
	}
	if err != nil {
		return CheckinResponse{}, false, err
	}
	return response, true, nil
}

func insertCheckinRejectionTx(ctx context.Context, tx pgx.Tx, actor Actor, ticketID string, eventID string, deviceID string, reason string, detail string) error {
	rejectionID, err := newID("rej")
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO checkin_rejections
		(rejection_id, ticket_id, event_id, staff_id, device_id, reason, detail, rejected_at)
		VALUES ($1,NULLIF($2, ''),$3,$4,$5,$6,$7,now())`,
		rejectionID, ticketID, eventID, actor.ID, deviceID, reason, detail)
	return err
}
