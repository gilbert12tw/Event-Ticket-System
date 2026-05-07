package ticketing

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Service) GetTicket(ctx context.Context, actor Actor, ticketID string) (Ticket, error) {
	var ticket Ticket
	err := s.db.QueryRow(ctx, `SELECT
			t.ticket_id, t.registration_id, t.event_id, t.employee_id, t.status, t.sequence_number,
			COALESCE(t.expires_at, t.issued_at + interval '24 hours'), t.revoked_reason, t.issued_at,
			e.title, e.location, e.starts_at, emp.full_name
		FROM tickets t
		JOIN events e ON e.event_id = t.event_id
		JOIN employees emp ON emp.employee_id = t.employee_id
		WHERE t.ticket_id = $1`, ticketID).
		Scan(&ticket.TicketID, &ticket.RegistrationID, &ticket.EventID, &ticket.EmployeeID, &ticket.Status, &ticket.SequenceNumber, &ticket.ExpiresAt, &ticket.RevokedReason, &ticket.IssuedAt, &ticket.EventTitle, &ticket.EventLocation, &ticket.EventStartsAt, &ticket.EmployeeName)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, notFound("ticket not found")
	}
	if err != nil {
		return Ticket{}, err
	}
	if actor.Role == RoleEmployee && actor.ID != ticket.EmployeeID {
		return Ticket{}, forbidden("employees may only view their own tickets")
	}
	if actor.Role != RoleEmployee {
		if err := requireAnyRole(actor, RoleActivityAdmin, RoleHRAdmin, RoleCheckinStaff); err != nil {
			return Ticket{}, err
		}
	}
	result, err := s.ticketForActor(actor, ticket)
	if err != nil {
		return Ticket{}, err
	}
	return result, nil
}
