package ticketing

import "context"

func (s *Service) ListTickets(ctx context.Context, actor Actor, employeeID string) ([]Ticket, error) {
	if err := requireRole(actor, RoleEmployee); err != nil {
		return nil, err
	}
	if employeeID == "" {
		employeeID = actor.ID
	}
	if actor.ID != "" && actor.ID != employeeID {
		return nil, forbidden("employees may only view their own tickets")
	}

	rows, err := s.db.Query(ctx, `SELECT `+ticketWithEventSelectColumns+`
		`+ticketWithEventFrom+`
		WHERE t.employee_id = $1
		ORDER BY t.issued_at DESC`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var ticket Ticket
		if err := scanTicketWithEventRow(rows, &ticket); err != nil {
			return nil, err
		}
		ticket, err = s.ticketForActor(actor, ticket)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}
	return tickets, rows.Err()
}
