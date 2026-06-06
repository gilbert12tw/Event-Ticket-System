package ticketing

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Service) GetTicket(ctx context.Context, actor Actor, ticketID string) (Ticket, error) {
	var ticket Ticket
	err := scanTicketWithEventRow(s.db.QueryRow(ctx, `SELECT `+ticketWithEventSelectColumns+`
		`+ticketWithEventFrom+`
		WHERE t.ticket_id = $1`, ticketID), &ticket)
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
