package ticketing

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	ticketSelectColumns = `t.ticket_id, t.registration_id, t.event_id, t.employee_id, t.status, t.sequence_number,
		COALESCE(t.expires_at, t.issued_at + interval '24 hours'), t.revoked_reason, t.issued_at,
		r.family_count, e.full_name, e.department, e.site`
	checkinTicketSelectColumns = `t.ticket_id, t.registration_id, t.event_id, t.employee_id, t.status, t.sequence_number,
		COALESCE(t.expires_at, t.issued_at + interval '24 hours'), t.revoked_reason, t.issued_at, r.family_count,
		ev.title, e.full_name, e.department, e.site`
	ticketWithEventSelectColumns = `t.ticket_id, t.registration_id, t.event_id, t.employee_id, t.status, t.sequence_number,
		COALESCE(t.expires_at, t.issued_at + interval '24 hours'), t.revoked_reason, t.issued_at, r.family_count,
		e.title, e.location, e.starts_at, emp.full_name, emp.department, emp.site`
	ticketWithEventFrom = `FROM tickets t
		JOIN events e ON e.event_id = t.event_id
		JOIN registrations r ON r.registration_id = t.registration_id
		JOIN employees emp ON emp.employee_id = t.employee_id`
)

type ticketQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *Service) createTicketTx(ctx context.Context, tx pgx.Tx, reg Registration, employee Employee) (Ticket, error) {
	ticketID, err := newID("tkt")
	if err != nil {
		return Ticket{}, err
	}
	issuedAt := s.now()
	token, err := s.signer.Sign(TicketClaims{TicketID: ticketID, EventID: reg.EventID, EmployeeID: reg.EmployeeID})
	if err != nil {
		return Ticket{}, err
	}
	tokenHash := s.signer.HashToken(token)
	qrPayload := token
	expiresAt := issuedAt.Add(30 * 24 * time.Hour)
	_ = tx.QueryRow(ctx, `SELECT starts_at + interval '24 hours' FROM events WHERE event_id = $1`, reg.EventID).Scan(&expiresAt)
	_, err = tx.Exec(ctx, `INSERT INTO tickets
		(ticket_id, registration_id, event_id, employee_id, status, sequence_number, signed_token_hash, signed_token, qr_payload, expires_at, issued_at)
		VALUES ($1,$2,$3,$4,'active',1,$5,$6,$7,$8,$9)`,
		ticketID, reg.RegistrationID, reg.EventID, reg.EmployeeID, tokenHash, "", "", expiresAt, issuedAt)
	if err != nil {
		return Ticket{}, err
	}
	return Ticket{
		TicketID:        ticketID,
		RegistrationID:  reg.RegistrationID,
		EventID:         reg.EventID,
		EmployeeID:      reg.EmployeeID,
		Status:          TicketActive,
		SequenceNumber:  1,
		SignedToken:     token,
		QRPayload:       qrPayload,
		ExpiresAt:       expiresAt,
		IssuedAt:        issuedAt,
		EmployeeName:    employee.FullName,
		Department:      employee.Department,
		City:            employee.Site,
		FamilyCount:     reg.FamilyCount,
		NonTransferable: true,
	}, nil
}

func (s *Service) findTicketByRegistration(ctx context.Context, registrationID string) (*Ticket, error) {
	return s.findTicketByRegistrationWith(ctx, s.db, registrationID)
}

func (s *Service) findTicketByRegistrationTx(ctx context.Context, tx pgx.Tx, registrationID string) (*Ticket, error) {
	return s.findTicketByRegistrationWith(ctx, tx, registrationID)
}

func (s *Service) findTicketByRegistrationWith(ctx context.Context, q ticketQuerier, registrationID string) (*Ticket, error) {
	var ticket Ticket
	err := scanTicketRow(q.QueryRow(ctx, `SELECT `+ticketSelectColumns+`
		FROM tickets t
		JOIN registrations r ON r.registration_id = t.registration_id
		JOIN employees e ON e.employee_id = t.employee_id
		WHERE t.registration_id = $1`, registrationID), &ticket)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.hydrateTicketToken(&ticket); err != nil {
		return nil, err
	}
	return &ticket, nil
}

func (s *Service) lockTicketTx(ctx context.Context, tx pgx.Tx, ticketID string) (Ticket, error) {
	return findTicketByIDTx(ctx, tx, ticketID, true)
}

func ticketSnapshotTx(ctx context.Context, tx pgx.Tx, ticketID string) (Ticket, error) {
	return findTicketByIDTx(ctx, tx, ticketID, false)
}

func findTicketByIDTx(ctx context.Context, tx pgx.Tx, ticketID string, lock bool) (Ticket, error) {
	var ticket Ticket
	query := `SELECT ` + ticketSelectColumns + `
		FROM tickets t
		JOIN registrations r ON r.registration_id = t.registration_id
		JOIN employees e ON e.employee_id = t.employee_id
		WHERE t.ticket_id = $1`
	if lock {
		query += ` FOR UPDATE OF t`
	}
	err := scanTicketRow(tx.QueryRow(ctx, query, ticketID), &ticket)
	if err == pgx.ErrNoRows {
		return Ticket{}, notFound("ticket not found")
	}
	return ticket, err
}

func scanTicketRow(row pgx.Row, ticket *Ticket) error {
	err := row.Scan(&ticket.TicketID, &ticket.RegistrationID, &ticket.EventID, &ticket.EmployeeID, &ticket.Status, &ticket.SequenceNumber, &ticket.ExpiresAt, &ticket.RevokedReason, &ticket.IssuedAt, &ticket.FamilyCount, &ticket.EmployeeName, &ticket.Department, &ticket.City)
	if err != nil {
		return err
	}
	ticket.NonTransferable = true
	return nil
}

func scanTicketWithEventRow(row pgx.Row, ticket *Ticket) error {
	err := row.Scan(&ticket.TicketID, &ticket.RegistrationID, &ticket.EventID, &ticket.EmployeeID, &ticket.Status, &ticket.SequenceNumber, &ticket.ExpiresAt, &ticket.RevokedReason, &ticket.IssuedAt, &ticket.FamilyCount, &ticket.EventTitle, &ticket.EventLocation, &ticket.EventStartsAt, &ticket.EmployeeName, &ticket.Department, &ticket.City)
	if err != nil {
		return err
	}
	ticket.NonTransferable = true
	return nil
}

func scanCheckinTicketRow(row pgx.Row, ticket *Ticket) error {
	err := row.Scan(&ticket.TicketID, &ticket.RegistrationID, &ticket.EventID, &ticket.EmployeeID, &ticket.Status, &ticket.SequenceNumber, &ticket.ExpiresAt, &ticket.RevokedReason, &ticket.IssuedAt, &ticket.FamilyCount, &ticket.EventTitle, &ticket.EmployeeName, &ticket.Department, &ticket.City)
	if err != nil {
		return err
	}
	ticket.NonTransferable = true
	return nil
}

func (s *Service) hydrateTicketToken(ticket *Ticket) error {
	if ticket == nil {
		return nil
	}
	token, err := s.signer.Sign(TicketClaims{
		TicketID:   ticket.TicketID,
		EventID:    ticket.EventID,
		EmployeeID: ticket.EmployeeID,
	})
	if err != nil {
		return err
	}
	ticket.SignedToken = token
	ticket.QRPayload = token
	return nil
}

func sanitizeTicket(ticket *Ticket) *Ticket {
	if ticket == nil {
		return nil
	}
	copy := *ticket
	copy.SignedToken = ""
	copy.QRPayload = ""
	return &copy
}

func insertTicketIssuedAuditTx(ctx context.Context, tx pgx.Tx, actor Actor, ticket Ticket) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	return insertAudit(ctx, tx, newAuditRecord(auditID, actor, "ticket.issued", "ticket", ticket.TicketID, map[string]interface{}{
		"event_id":        ticket.EventID,
		"registration_id": ticket.RegistrationID,
		"employee_id":     ticket.EmployeeID,
	}))
}

func (s *Service) ticketForActor(actor Actor, ticket Ticket) (Ticket, error) {
	if err := requireAnyRole(actor, RoleEmployee, RoleActivityAdmin, RoleHRAdmin, RoleCheckinStaff); err != nil {
		return Ticket{}, err
	}
	if actor.Role == RoleEmployee {
		if actor.ID != ticket.EmployeeID {
			return Ticket{}, forbidden("employees may only view their own tickets")
		}
		if err := s.hydrateTicketToken(&ticket); err != nil {
			return Ticket{}, err
		}
		return ticket, nil
	}
	return *sanitizeTicket(&ticket), nil
}
