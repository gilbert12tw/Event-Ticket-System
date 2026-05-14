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

	rows, err := s.db.Query(ctx, `SELECT
			t.ticket_id, t.registration_id, t.event_id, t.employee_id, t.status, t.sequence_number,
			COALESCE(t.expires_at, t.issued_at + interval '24 hours'), t.revoked_reason, t.issued_at, r.family_count,
			e.title, e.location, e.starts_at, emp.full_name, emp.department, emp.site
		FROM tickets t
		JOIN events e ON e.event_id = t.event_id
		JOIN registrations r ON r.registration_id = t.registration_id
		JOIN employees emp ON emp.employee_id = t.employee_id
		WHERE t.employee_id = $1
		ORDER BY t.issued_at DESC`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var ticket Ticket
		if err := rows.Scan(&ticket.TicketID, &ticket.RegistrationID, &ticket.EventID, &ticket.EmployeeID, &ticket.Status, &ticket.SequenceNumber, &ticket.ExpiresAt, &ticket.RevokedReason, &ticket.IssuedAt, &ticket.FamilyCount, &ticket.EventTitle, &ticket.EventLocation, &ticket.EventStartsAt, &ticket.EmployeeName, &ticket.Department, &ticket.City); err != nil {
			return nil, err
		}
		ticket.NonTransferable = true
		ticket, err = s.ticketForActor(actor, ticket)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
	}
	return tickets, rows.Err()
}
