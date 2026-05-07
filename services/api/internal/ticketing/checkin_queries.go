package ticketing

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Service) findCheckinByTicketTx(ctx context.Context, tx pgx.Tx, ticketID string) (CheckinResponse, bool, error) {
	var response CheckinResponse
	err := tx.QueryRow(ctx, `SELECT c.checkin_id, c.ticket_id, t.event_id, t.employee_id, c.status, c.scanned_at, c.staff_id
		FROM checkin_records c
		JOIN tickets t ON t.ticket_id = c.ticket_id
		WHERE c.ticket_id = $1`, ticketID).
		Scan(&response.CheckinID, &response.TicketID, &response.EventID, &response.EmployeeID, &response.Status, &response.ScannedAt, &response.FirstScannedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return CheckinResponse{}, false, nil
	}
	if err != nil {
		return CheckinResponse{}, false, err
	}
	return response, true, nil
}
