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
		if auditErr := s.insertCheckinRejectionAudit(ctx, actor, "", "", req.DeviceID, "invalid_ticket_token", ""); auditErr != nil {
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
	err = tx.QueryRow(ctx, `SELECT t.ticket_id, t.registration_id, t.event_id, t.employee_id, t.status, t.sequence_number,
			COALESCE(t.expires_at, t.issued_at + interval '24 hours'), t.revoked_reason, t.issued_at, r.family_count,
			e.full_name, e.department, e.site
		FROM tickets t
		JOIN registrations r ON r.registration_id = t.registration_id
		JOIN employees e ON e.employee_id = t.employee_id
		WHERE t.signed_token_hash = $1 FOR UPDATE OF t`, tokenHash).
		Scan(&ticket.TicketID, &ticket.RegistrationID, &ticket.EventID, &ticket.EmployeeID, &ticket.Status, &ticket.SequenceNumber,
			&ticket.ExpiresAt, &ticket.RevokedReason, &ticket.IssuedAt, &ticket.FamilyCount, &ticket.EmployeeName, &ticket.Department, &ticket.City)
	if errors.Is(err, pgx.ErrNoRows) {
		if auditErr := insertCheckinRejectionAuditTx(ctx, tx, actor, "", claims.EventID, req.DeviceID, "ticket_not_found", ""); auditErr != nil {
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
	holder := ticketHolderFromTicket(ticket)
	if claims.TicketID != ticket.TicketID || claims.EventID != ticket.EventID || claims.EmployeeID != ticket.EmployeeID {
		if err := insertCheckinRejectionAuditTx(ctx, tx, actor, ticket.TicketID, ticket.EventID, req.DeviceID, "ticket_token_claims_mismatch", ""); err != nil {
			return CheckinResponse{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CheckinResponse{}, err
		}
		return CheckinResponse{
			TicketID:         ticket.TicketID,
			EventID:          ticket.EventID,
			EmployeeID:       ticket.EmployeeID,
			Status:           "rejected",
			ReasonCode:       "ticket_token_claims_mismatch",
			ScannedAt:        s.now(),
			ConflictReason:   "ticket_token_claims_mismatch",
			RejectionMessage: "ticket token claims do not match",
			Holder:           holder,
			FamilyCount:      ticket.FamilyCount,
		}, badRequest("ticket token claims do not match")
	}
	if requestEventID := strings.TrimSpace(req.EventID); requestEventID != "" && requestEventID != ticket.EventID {
		if err := insertCheckinRejectionAuditTx(ctx, tx, actor, ticket.TicketID, ticket.EventID, req.DeviceID, "event_mismatch", requestEventID); err != nil {
			return CheckinResponse{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CheckinResponse{}, err
		}
		return CheckinResponse{
			TicketID:         ticket.TicketID,
			EventID:          ticket.EventID,
			EmployeeID:       ticket.EmployeeID,
			Status:           "rejected",
			ReasonCode:       "event_mismatch",
			ScannedAt:        s.now(),
			ConflictReason:   "event_mismatch",
			RejectionMessage: "ticket belongs to a different event",
			Holder:           holder,
			FamilyCount:      ticket.FamilyCount,
		}, conflict("ticket belongs to a different event")
	}
	if reason := strings.TrimSpace(req.HolderMismatchReason); reason != "" {
		if err := insertCheckinRejectionAuditTx(ctx, tx, actor, ticket.TicketID, ticket.EventID, req.DeviceID, "holder_mismatch", reason); err != nil {
			return CheckinResponse{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CheckinResponse{}, err
		}
		return CheckinResponse{
			TicketID:         ticket.TicketID,
			EventID:          ticket.EventID,
			EmployeeID:       ticket.EmployeeID,
			Status:           "rejected",
			ReasonCode:       "holder_mismatch",
			ScannedAt:        s.now(),
			ConflictReason:   "holder_mismatch",
			RejectionMessage: reason,
			Holder:           holder,
			FamilyCount:      ticket.FamilyCount,
		}, conflict("ticket holder mismatch")
	}

	existing, found, err := s.findCheckinByTicketTx(ctx, tx, ticket.TicketID)
	if err != nil {
		return CheckinResponse{}, err
	}
	if found {
		existing.Duplicate = true
		existing.FirstScannedAt = existing.ScannedAt
		existing.ReasonCode = "duplicate_scan"
		existing.ConflictReason = "ticket_already_redeemed"
		if err := insertCheckinRejectionTx(ctx, tx, actor, ticket.TicketID, ticket.EventID, req.DeviceID, "ticket_already_redeemed", existing.CheckinID); err != nil {
			return CheckinResponse{}, err
		}
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
		reason := "ticket_not_active"
		message := "ticket is not active"
		if ticket.Status == TicketRevoked {
			reason = "revoked_ticket"
			message = "ticket is revoked"
		}
		if err := insertCheckinRejectionAuditTx(ctx, tx, actor, ticket.TicketID, ticket.EventID, req.DeviceID, reason, ""); err != nil {
			return CheckinResponse{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CheckinResponse{}, err
		}
		return CheckinResponse{
			TicketID:         ticket.TicketID,
			EventID:          ticket.EventID,
			EmployeeID:       ticket.EmployeeID,
			Status:           "rejected",
			ReasonCode:       reason,
			ScannedAt:        s.now(),
			ConflictReason:   reason,
			RejectionMessage: message,
			Holder:           holder,
			FamilyCount:      ticket.FamilyCount,
		}, conflict(message)
	}
	if !ticket.ExpiresAt.IsZero() && s.now().After(ticket.ExpiresAt) {
		if err := s.expireTicketTx(ctx, tx, actor, ticket, req.DeviceID); err != nil {
			return CheckinResponse{}, err
		}
		if err := insertCheckinRejectionAuditTx(ctx, tx, actor, ticket.TicketID, ticket.EventID, req.DeviceID, "expired_ticket", ""); err != nil {
			return CheckinResponse{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CheckinResponse{}, err
		}
		return CheckinResponse{
			TicketID:         ticket.TicketID,
			EventID:          ticket.EventID,
			EmployeeID:       ticket.EmployeeID,
			Status:           "rejected",
			ReasonCode:       "expired_ticket",
			ScannedAt:        s.now(),
			ConflictReason:   "expired_ticket",
			RejectionMessage: "ticket is expired",
			Holder:           holder,
			FamilyCount:      ticket.FamilyCount,
		}, conflict("ticket is expired")
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
		CheckinID:   checkinID,
		TicketID:    ticket.TicketID,
		EventID:     ticket.EventID,
		EmployeeID:  ticket.EmployeeID,
		Status:      "accepted",
		ReasonCode:  "accepted",
		ScannedAt:   scannedAt,
		Holder:      holder,
		FamilyCount: ticket.FamilyCount,
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

func (s *Service) insertCheckinRejectionAudit(ctx context.Context, actor Actor, ticketID string, eventID string, deviceID string, reason string, detail string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	if err := insertCheckinRejectionAuditTx(ctx, tx, actor, ticketID, eventID, deviceID, reason, detail); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func insertCheckinRejectionAuditTx(ctx context.Context, tx pgx.Tx, actor Actor, ticketID string, eventID string, deviceID string, reason string, detail string) error {
	if err := insertCheckinRejectionTx(ctx, tx, actor, ticketID, eventID, deviceID, reason, detail); err != nil {
		return err
	}
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
	metadata := map[string]interface{}{
		"event_id":  eventID,
		"device_id": deviceID,
		"reason":    reason,
	}
	if detail != "" {
		metadata["detail"] = detail
	}
	return insertAudit(ctx, tx, auditID, actor, "checkin.rejected", entityType, entityID, metadata)
}
