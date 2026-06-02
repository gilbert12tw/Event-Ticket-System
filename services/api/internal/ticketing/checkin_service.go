package ticketing

import (
	"context"
	"strings"

	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
)

const ticketRedeemedEventType = "ticket.redeemed"

func (s *Service) CheckIn(ctx context.Context, actor Actor, req CheckinRequest) (CheckinResponse, error) {
	if err := requireRole(actor, RoleCheckinStaff); err != nil {
		return CheckinResponse{}, err
	}
	req, err := normalizeCheckinRequest(req)
	if err != nil {
		return CheckinResponse{}, err
	}
	claims, err := s.signer.Verify(req.SignedToken)
	if err != nil {
		if auditErr := s.insertCheckinRejectionAudit(ctx, actor, newCheckinRejection("", req.EventID, req.DeviceID, "invalid_ticket_token", "")); auditErr != nil {
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

	ticket, found, err := scanCheckinTicketByHashTx(ctx, tx, tokenHash)
	if err != nil {
		return CheckinResponse{}, err
	}
	if !found {
		return rejectMissingCheckinTicketTx(ctx, tx, actor, req)
	}
	if result, handled, err := s.rejectInvalidCheckinBeforeRedeemTx(ctx, tx, actor, req, claims, ticket); handled {
		return result, err
	}
	return s.redeemCheckinTicketTx(ctx, tx, actor, req, ticket)
}

func normalizeCheckinRequest(req CheckinRequest) (CheckinRequest, error) {
	req.SignedToken = strings.TrimSpace(req.SignedToken)
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	req.EventID = strings.TrimSpace(req.EventID)
	switch {
	case req.SignedToken == "":
		return CheckinRequest{}, badRequest("signed_token is required")
	case req.DeviceID == "":
		return CheckinRequest{}, badRequest("device_id is required")
	case req.EventID == "":
		return CheckinRequest{}, badRequest("event_id is required")
	}
	return req, nil
}

func scanCheckinTicketByHashTx(ctx context.Context, tx pgx.Tx, tokenHash string) (Ticket, bool, error) {
	return scanLockedTicketByHashTx(ctx, tx, tokenHash)
}

func rejectMissingCheckinTicketTx(ctx context.Context, tx pgx.Tx, actor Actor, req CheckinRequest) (CheckinResponse, error) {
	if auditErr := insertCheckinRejectionAuditTx(ctx, tx, actor, newCheckinRejection("", req.EventID, req.DeviceID, "ticket_not_found", "")); auditErr != nil {
		return CheckinResponse{}, auditErr
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return CheckinResponse{}, commitErr
	}
	return CheckinResponse{}, badRequest("invalid ticket token")
}

func (s *Service) rejectInvalidCheckinBeforeRedeemTx(ctx context.Context, tx pgx.Tx, actor Actor, req CheckinRequest, claims TicketClaims, ticket Ticket) (CheckinResponse, bool, error) {
	if !claimsMatchTicket(claims, ticket) {
		return s.rejectCheckinTicketTx(ctx, tx, actor, newCheckinTicketRejection(req, ticket, "ticket_token_claims_mismatch", "", "ticket token claims do not match", badRequest("ticket token claims do not match")))
	}
	if req.EventID != ticket.EventID {
		return s.rejectCheckinTicketTx(ctx, tx, actor, newCheckinTicketRejection(req, ticket, "event_mismatch", req.EventID, "ticket belongs to a different event", conflict("ticket belongs to a different event")))
	}
	if reason := strings.TrimSpace(req.HolderMismatchReason); reason != "" {
		return s.rejectCheckinTicketTx(ctx, tx, actor, newCheckinTicketRejection(req, ticket, "holder_mismatch", reason, reason, conflict("ticket holder mismatch")))
	}
	if result, found, err := s.rejectDuplicateCheckinTx(ctx, tx, actor, req, ticket); err != nil || found {
		return result, found, err
	}
	if ticket.Status != TicketActive {
		return s.rejectInactiveTicketTx(ctx, tx, actor, req, ticket)
	}
	if !ticket.ExpiresAt.IsZero() && s.now().After(ticket.ExpiresAt) {
		return s.rejectExpiredTicketTx(ctx, tx, actor, req, ticket)
	}
	return CheckinResponse{}, false, nil
}

type checkinTicketRejection struct {
	req         CheckinRequest
	ticket      Ticket
	reason      string
	detail      string
	message     string
	responseErr error
}

func newCheckinTicketRejection(req CheckinRequest, ticket Ticket, reason string, detail string, message string, responseErr error) checkinTicketRejection {
	return checkinTicketRejection{req: req, ticket: ticket, reason: reason, detail: detail, message: message, responseErr: responseErr}
}

func (s *Service) rejectCheckinTicketTx(ctx context.Context, tx pgx.Tx, actor Actor, rejection checkinTicketRejection) (CheckinResponse, bool, error) {
	if err := insertCheckinRejectionAuditTx(ctx, tx, actor, newCheckinRejection(rejection.ticket.TicketID, rejection.ticket.EventID, rejection.req.DeviceID, rejection.reason, rejection.detail)); err != nil {
		return CheckinResponse{}, true, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CheckinResponse{}, true, err
	}
	return s.rejectedCheckinResponse(rejection.ticket, rejection.reason, rejection.message), true, rejection.responseErr
}

func (s *Service) rejectDuplicateCheckinTx(ctx context.Context, tx pgx.Tx, actor Actor, req CheckinRequest, ticket Ticket) (CheckinResponse, bool, error) {
	existing, found, err := s.findCheckinByTicketTx(ctx, tx, ticket.TicketID)
	if err != nil || !found {
		return CheckinResponse{}, found, err
	}
	existing.Duplicate = true
	existing.FirstScannedAt = existing.ScannedAt
	existing.ReasonCode = "duplicate_scan"
	existing.ConflictReason = "ticket_already_redeemed"
	if err := insertCheckinRejectionTx(ctx, tx, actor, newCheckinRejection(ticket.TicketID, ticket.EventID, req.DeviceID, "ticket_already_redeemed", existing.CheckinID)); err != nil {
		return CheckinResponse{}, true, err
	}
	auditID, err := newID("aud")
	if err != nil {
		return CheckinResponse{}, true, err
	}
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor, "checkin.conflict", "ticket", ticket.TicketID, map[string]interface{}{"event_id": ticket.EventID, "device_id": req.DeviceID, "first_checkin_id": existing.CheckinID})); err != nil {
		return CheckinResponse{}, true, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CheckinResponse{}, true, err
	}
	return existing, true, conflict("ticket has already been redeemed")
}

func (s *Service) rejectInactiveTicketTx(ctx context.Context, tx pgx.Tx, actor Actor, req CheckinRequest, ticket Ticket) (CheckinResponse, bool, error) {
	reason := "ticket_not_active"
	message := "ticket is not active"
	if ticket.Status == TicketRevoked {
		reason = "revoked_ticket"
		message = "ticket is revoked"
	}
	return s.rejectCheckinTicketTx(ctx, tx, actor, newCheckinTicketRejection(req, ticket, reason, "", message, conflict(message)))
}

func (s *Service) rejectExpiredTicketTx(ctx context.Context, tx pgx.Tx, actor Actor, req CheckinRequest, ticket Ticket) (CheckinResponse, bool, error) {
	if err := s.expireTicketTx(ctx, tx, actor, ticket, req.DeviceID); err != nil {
		return CheckinResponse{}, true, err
	}
	return s.rejectCheckinTicketTx(ctx, tx, actor, newCheckinTicketRejection(req, ticket, "expired_ticket", "", "ticket is expired", conflict("ticket is expired")))
}

func (s *Service) redeemCheckinTicketTx(ctx context.Context, tx pgx.Tx, actor Actor, req CheckinRequest, ticket Ticket) (CheckinResponse, error) {
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
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor, ticketRedeemedEventType, "ticket", ticket.TicketID, map[string]interface{}{"event_id": ticket.EventID, "device_id": req.DeviceID})); err != nil {
		return CheckinResponse{}, err
	}
	if err := insertOutbox(ctx, tx, ticketRedeemedEventType, ticket.TicketID, ticketOutboxPayload(ticket, nil)); err != nil {
		return CheckinResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CheckinResponse{}, err
	}

	s.logger.Info("ticket redeemed", "trace_id", traceid.FromContext(ctx), "action", ticketRedeemedEventType, "status", "success", "event_id", ticket.EventID, "ticket_id", ticket.TicketID)
	return CheckinResponse{
		CheckinID:   checkinID,
		TicketID:    ticket.TicketID,
		EventID:     ticket.EventID,
		EventTitle:  ticket.EventTitle,
		EmployeeID:  ticket.EmployeeID,
		Status:      "accepted",
		ReasonCode:  "accepted",
		ScannedAt:   scannedAt,
		Holder:      ticketHolderFromTicket(ticket),
		FamilyCount: ticket.FamilyCount,
	}, nil
}

func (s *Service) rejectedCheckinResponse(ticket Ticket, reasonCode string, message string) CheckinResponse {
	return CheckinResponse{
		TicketID:         ticket.TicketID,
		EventID:          ticket.EventID,
		EventTitle:       ticket.EventTitle,
		EmployeeID:       ticket.EmployeeID,
		Status:           "rejected",
		ReasonCode:       reasonCode,
		ScannedAt:        s.now(),
		ConflictReason:   reasonCode,
		RejectionMessage: message,
		Holder:           ticketHolderFromTicket(ticket),
		FamilyCount:      ticket.FamilyCount,
	}
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
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor, "ticket.expired", "ticket", ticket.TicketID, map[string]interface{}{"event_id": ticket.EventID, "device_id": deviceID})); err != nil {
		return err
	}
	return insertOutbox(ctx, tx, "ticket.expired", ticket.TicketID, ticketOutboxPayload(ticket, nil))
}

func (s *Service) insertCheckinRejectionAudit(ctx context.Context, actor Actor, rejection checkinRejection) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	if err := insertCheckinRejectionAuditTx(ctx, tx, actor, rejection); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func insertCheckinRejectionAuditTx(ctx context.Context, tx pgx.Tx, actor Actor, rejection checkinRejection) error {
	if err := insertCheckinRejectionTx(ctx, tx, actor, rejection); err != nil {
		return err
	}
	entityType := "checkin_attempt"
	entityID := rejection.deviceID
	if rejection.ticketID != "" {
		entityType = "ticket"
		entityID = rejection.ticketID
	}
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	metadata, err := eventContextMetadataTx(ctx, tx, rejection.eventID)
	if err != nil {
		return err
	}
	metadata = mergeMetadata(metadata, map[string]interface{}{
		"device_id": rejection.deviceID,
		"reason":    rejection.reason,
	})
	if rejection.detail != "" {
		if rejection.reason == "event_mismatch" {
			metadata["requested_event_id"] = rejection.detail
		}
	}
	return insertAudit(ctx, tx, newAuditRecord(auditID, actor, "checkin.rejected", entityType, entityID, metadata))
}
