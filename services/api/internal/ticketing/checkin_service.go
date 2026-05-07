package ticketing

import (
	"context"
	"errors"
	"strings"

	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
)

func (s *Service) CheckIn(ctx context.Context, actor Actor, req CheckinRequest) (CheckinResponse, error) {
	if err := requireRole(actor, RoleCheckinStaff); err != nil {
		return CheckinResponse{}, err
	}
	if strings.TrimSpace(req.SignedToken) == "" {
		return CheckinResponse{}, badRequest("signed_token is required")
	}
	if strings.TrimSpace(req.DeviceID) == "" {
		return CheckinResponse{}, badRequest("device_id is required")
	}
	claims, err := s.signer.Verify(req.SignedToken)
	if err != nil {
		if auditErr := s.insertCheckinRejectionAudit(ctx, actor, "", "", req.DeviceID, "invalid_ticket_token"); auditErr != nil {
			return CheckinResponse{}, auditErr
		}
		return CheckinResponse{}, badRequest("invalid ticket token")
	}
	tokenHash := s.signer.HashToken(req.SignedToken)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CheckinResponse{}, err
	}
	defer rollback(ctx, tx)

	var ticket Ticket
	err = tx.QueryRow(ctx, `SELECT ticket_id, registration_id, event_id, employee_id, status, sequence_number, COALESCE(expires_at, issued_at + interval '24 hours'), revoked_reason, issued_at
		FROM tickets WHERE signed_token_hash = $1 FOR UPDATE`, tokenHash).
		Scan(&ticket.TicketID, &ticket.RegistrationID, &ticket.EventID, &ticket.EmployeeID, &ticket.Status, &ticket.SequenceNumber, &ticket.ExpiresAt, &ticket.RevokedReason, &ticket.IssuedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		if auditErr := insertCheckinRejectionAuditTx(ctx, tx, actor, "", claims.EventID, req.DeviceID, "ticket_not_found"); auditErr != nil {
			return CheckinResponse{}, auditErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return CheckinResponse{}, commitErr
		}
		return CheckinResponse{}, badRequest("invalid ticket token")
	}
	if err != nil {
		return CheckinResponse{}, err
	}
	if claims.TicketID != ticket.TicketID || claims.EventID != ticket.EventID || claims.EmployeeID != ticket.EmployeeID {
		if err := insertCheckinRejectionAuditTx(ctx, tx, actor, ticket.TicketID, ticket.EventID, req.DeviceID, "ticket_token_claims_mismatch"); err != nil {
			return CheckinResponse{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CheckinResponse{}, err
		}
		return CheckinResponse{}, badRequest("ticket token claims do not match")
	}

	existing, found, err := s.findCheckinByTicketTx(ctx, tx, ticket.TicketID)
	if err != nil {
		return CheckinResponse{}, err
	}
	if found {
		existing.Duplicate = true
		existing.FirstScannedAt = existing.ScannedAt
		auditID, err := newID("aud")
		if err != nil {
			return CheckinResponse{}, err
		}
		if err := insertAudit(ctx, tx, auditID, actor, "checkin.conflict", "ticket", ticket.TicketID, map[string]interface{}{"event_id": ticket.EventID, "device_id": req.DeviceID, "first_checkin_id": existing.CheckinID}); err != nil {
			return CheckinResponse{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CheckinResponse{}, err
		}
		return existing, conflict("ticket has already been redeemed")
	}
	if ticket.Status != TicketActive {
		if err := insertCheckinRejectionAuditTx(ctx, tx, actor, ticket.TicketID, ticket.EventID, req.DeviceID, "ticket_not_active"); err != nil {
			return CheckinResponse{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CheckinResponse{}, err
		}
		return CheckinResponse{}, conflict("ticket is not active")
	}
	if !ticket.ExpiresAt.IsZero() && s.now().After(ticket.ExpiresAt) {
		if err := s.expireTicketTx(ctx, tx, actor, ticket, req.DeviceID); err != nil {
			return CheckinResponse{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CheckinResponse{}, err
		}
		return CheckinResponse{}, conflict("ticket is expired")
	}

	checkinID, err := newID("chk")
	if err != nil {
		return CheckinResponse{}, err
	}
	scannedAt := s.now()
	_, err = tx.Exec(ctx, `INSERT INTO checkin_records (checkin_id, ticket_id, staff_id, device_id, status, scanned_at)
		VALUES ($1,$2,$3,$4,'accepted',$5)`, checkinID, ticket.TicketID, actor.ID, req.DeviceID, scannedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return CheckinResponse{}, conflict("ticket has already been redeemed")
		}
		return CheckinResponse{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE tickets SET status = 'redeemed' WHERE ticket_id = $1`, ticket.TicketID)
	if err != nil {
		return CheckinResponse{}, err
	}
	auditID, err := newID("aud")
	if err != nil {
		return CheckinResponse{}, err
	}
	if err := insertAudit(ctx, tx, auditID, actor, "ticket.redeemed", "ticket", ticket.TicketID, map[string]interface{}{"event_id": ticket.EventID, "device_id": req.DeviceID}); err != nil {
		return CheckinResponse{}, err
	}
	if err := insertOutbox(ctx, tx, "ticket.redeemed", ticket.TicketID, map[string]interface{}{"ticket_id": ticket.TicketID, "event_id": ticket.EventID}); err != nil {
		return CheckinResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CheckinResponse{}, err
	}

	s.logger.Info("ticket redeemed", "trace_id", traceid.FromContext(ctx), "action", "ticket.redeemed", "status", "success", "event_id", ticket.EventID, "ticket_id", ticket.TicketID)
	return CheckinResponse{
		CheckinID:  checkinID,
		TicketID:   ticket.TicketID,
		EventID:    ticket.EventID,
		EmployeeID: ticket.EmployeeID,
		Status:     "accepted",
		ScannedAt:  scannedAt,
	}, nil
}

func (s *Service) expireTicketTx(ctx context.Context, tx pgx.Tx, actor Actor, ticket Ticket, deviceID string) error {
	_, err := tx.Exec(ctx, `UPDATE tickets SET status = 'expired', revoked_reason = 'ticket expired' WHERE ticket_id = $1 AND status = 'active'`, ticket.TicketID)
	if err != nil {
		return err
	}
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, auditID, actor, "ticket.expired", "ticket", ticket.TicketID, map[string]interface{}{"event_id": ticket.EventID, "device_id": deviceID}); err != nil {
		return err
	}
	return insertOutbox(ctx, tx, "ticket.expired", ticket.TicketID, map[string]interface{}{"ticket_id": ticket.TicketID, "event_id": ticket.EventID})
}

func (s *Service) insertCheckinRejectionAudit(ctx context.Context, actor Actor, ticketID string, eventID string, deviceID string, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	if err := insertCheckinRejectionAuditTx(ctx, tx, actor, ticketID, eventID, deviceID, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func insertCheckinRejectionAuditTx(ctx context.Context, tx pgx.Tx, actor Actor, ticketID string, eventID string, deviceID string, reason string) error {
	entityType := "checkin_attempt"
	entityID := deviceID
	if ticketID != "" {
		entityType = "ticket"
		entityID = ticketID
	}
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	return insertAudit(ctx, tx, auditID, actor, "checkin.rejected", entityType, entityID, map[string]interface{}{
		"event_id":  eventID,
		"device_id": deviceID,
		"reason":    reason,
	})
}
