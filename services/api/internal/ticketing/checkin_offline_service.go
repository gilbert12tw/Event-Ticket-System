package ticketing

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	offlineScanStatusAccepted  = "accepted"
	offlineScanStatusConflict  = "conflict"
	offlineScanStatusDuplicate = "duplicate"

	offlineConflictClaimsMismatch = "ticket_token_claims_mismatch"
	offlineConflictEventMismatch  = "offline_scan_event_mismatch"
	offlineConflictExpired        = "ticket_expired"
	offlineConflictInvalidToken   = "invalid_ticket_token"
	offlineConflictNotActive      = "ticket_not_active"
	offlineConflictNotFound       = "ticket_not_found"
	offlineConflictRedeemed       = "ticket_already_redeemed"
)

func (s *Service) OfflineCheckinPackage(ctx context.Context, actor Actor, eventID string, deviceID string) (OfflineCheckinPackage, error) {
	if err := requireRole(actor, RoleCheckinStaff); err != nil {
		return OfflineCheckinPackage{}, err
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return OfflineCheckinPackage{}, badRequest("device_id is required")
	}
	rows, err := s.db.Query(ctx, `SELECT t.ticket_id, t.employee_id, t.signed_token_hash, r.family_count, e.full_name, e.department, e.site
		FROM tickets t
		JOIN registrations r ON r.registration_id = t.registration_id
		JOIN employees e ON e.employee_id = t.employee_id
		WHERE t.event_id = $1 AND t.status = 'active'
		ORDER BY t.issued_at ASC`, eventID)
	if err != nil {
		return OfflineCheckinPackage{}, err
	}
	defer rows.Close()
	var tickets []OfflineTicket
	for rows.Next() {
		var ticket OfflineTicket
		if err := rows.Scan(&ticket.TicketID, &ticket.EmployeeID, &ticket.TokenHash, &ticket.FamilyCount, &ticket.Holder.DisplayName, &ticket.Holder.Department, &ticket.Holder.City); err != nil {
			return OfflineCheckinPackage{}, err
		}
		tickets = append(tickets, ticket)
	}
	if err := rows.Err(); err != nil {
		return OfflineCheckinPackage{}, err
	}
	batchID, err := newID("off")
	if err != nil {
		return OfflineCheckinPackage{}, err
	}
	validUntil := s.now().Add(4 * time.Hour).UTC().Truncate(time.Second)
	signature, err := s.signer.SignOfflinePackage(OfflinePackageClaims{
		BatchID:    batchID,
		EventID:    eventID,
		DeviceID:   deviceID,
		StaffID:    actor.ID,
		ValidUntil: validUntil,
	})
	if err != nil {
		return OfflineCheckinPackage{}, err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO offline_checkin_batches
		(batch_id, event_id, device_id, staff_id, status, valid_until, package_signature)
		VALUES ($1,$2,$3,$4,'open',$5,$6)`, batchID, eventID, deviceID, actor.ID, validUntil, signature)
	if err != nil {
		return OfflineCheckinPackage{}, err
	}
	return OfflineCheckinPackage{
		BatchID:          batchID,
		EventID:          eventID,
		DeviceID:         deviceID,
		ValidUntil:       validUntil,
		PackageSignature: signature,
		TicketCount:      len(tickets),
		Tickets:          tickets,
	}, nil
}

func (s *Service) SyncOfflineCheckins(ctx context.Context, actor Actor, req OfflineCheckinSyncRequest) (OfflineCheckinSyncResponse, error) {
	if err := requireRole(actor, RoleCheckinStaff); err != nil {
		return OfflineCheckinSyncResponse{}, err
	}
	req.BatchID = strings.TrimSpace(req.BatchID)
	req.EventID = strings.TrimSpace(req.EventID)
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	req.PackageSignature = strings.TrimSpace(req.PackageSignature)
	if req.BatchID == "" {
		return OfflineCheckinSyncResponse{}, badRequest("batch_id is required")
	}
	if req.EventID == "" {
		return OfflineCheckinSyncResponse{}, badRequest("event_id is required")
	}
	if req.DeviceID == "" {
		return OfflineCheckinSyncResponse{}, badRequest("device_id is required")
	}
	if req.PackageSignature == "" {
		return OfflineCheckinSyncResponse{}, badRequest("package_signature is required")
	}
	if err := s.validateOfflineBatch(ctx, actor, req); err != nil {
		return OfflineCheckinSyncResponse{}, err
	}
	response := OfflineCheckinSyncResponse{BatchID: req.BatchID}
	for _, scan := range req.Scans {
		result, status, err := s.syncOfflineScan(ctx, actor, req, scan)
		if err != nil {
			return OfflineCheckinSyncResponse{}, err
		}
		response.Results = append(response.Results, result)
		switch status {
		case offlineScanStatusAccepted:
			response.Accepted++
		case offlineScanStatusDuplicate:
			response.Duplicate++
		default:
			response.Conflict++
		}
	}
	_, err := s.db.Exec(ctx, `UPDATE offline_checkin_batches SET status = CASE WHEN $2 > 0 THEN 'conflict' ELSE 'synced' END, synced_at = now() WHERE batch_id = $1`, req.BatchID, response.Conflict)
	return response, err
}

func (s *Service) validateOfflineBatch(ctx context.Context, actor Actor, req OfflineCheckinSyncRequest) error {
	var eventID, deviceID, staffID, status string
	var validUntil time.Time
	var packageSignature string
	err := s.db.QueryRow(ctx, `SELECT event_id, device_id, staff_id, status, valid_until, package_signature
		FROM offline_checkin_batches WHERE batch_id = $1`, req.BatchID).
		Scan(&eventID, &deviceID, &staffID, &status, &validUntil, &packageSignature)
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound("offline check-in batch not found")
	}
	if err != nil {
		return err
	}
	if staffID != actor.ID {
		return forbidden("offline check-in batch belongs to a different staff member")
	}
	if eventID != req.EventID {
		return conflict("offline check-in batch event mismatch")
	}
	if deviceID != req.DeviceID {
		return conflict("offline check-in batch device mismatch")
	}
	if status != "open" {
		return conflict("offline check-in batch is already synced")
	}
	if packageSignature != req.PackageSignature {
		return badRequest("offline package signature does not match batch")
	}
	claims, err := s.signer.VerifyOfflinePackage(req.PackageSignature)
	if err != nil {
		return badRequest("invalid offline package signature")
	}
	if claims.BatchID != req.BatchID || claims.EventID != req.EventID || claims.DeviceID != req.DeviceID || claims.StaffID != actor.ID {
		return badRequest("offline package claims do not match request")
	}
	if !claims.ValidUntil.Equal(validUntil.UTC()) {
		return badRequest("offline package expiry does not match batch")
	}
	if !validUntil.After(s.now()) {
		return conflict("offline package is expired")
	}
	return nil
}

func (s *Service) syncOfflineScan(ctx context.Context, actor Actor, req OfflineCheckinSyncRequest, scan OfflineCheckinScanInput) (CheckinResponse, string, error) {
	if scan.ScannedAt.IsZero() {
		scan.ScannedAt = s.now()
	}
	claims, err := s.signer.Verify(scan.SignedToken)
	if err != nil {
		return s.recordOfflineUnknownConflict(ctx, actor, req, scan, TicketClaims{}, offlineConflictInvalidToken)
	}
	tokenHash := s.signer.HashToken(scan.SignedToken)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
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
		result := conflictResultFromClaims(req, claims, scan.ScannedAt, offlineConflictNotFound)
		if err := s.insertOfflineScanTx(ctx, tx, req, "", offlineScanStatusConflict, scan.ScannedAt, tokenHash, result.ConflictReason); err != nil {
			return CheckinResponse{}, offlineScanStatusConflict, err
		}
		if err := s.insertOfflineConflictAuditTx(ctx, tx, actor, req, "offline_checkin_batch", req.BatchID, result.EventID, offlineScanStatusConflict, result.ConflictReason); err != nil {
			return CheckinResponse{}, offlineScanStatusConflict, err
		}
		return result, offlineScanStatusConflict, tx.Commit(ctx)
	}
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	if !claimsMatchTicket(claims, ticket) {
		return s.recordKnownOfflineConflict(ctx, tx, actor, req, ticket, scan.ScannedAt, tokenHash, offlineConflictClaimsMismatch)
	}
	if ticket.EventID != req.EventID {
		return s.recordKnownOfflineConflict(ctx, tx, actor, req, ticket, scan.ScannedAt, tokenHash, offlineConflictEventMismatch)
	}
	existing, found, err := s.findCheckinByTicketTx(ctx, tx, ticket.TicketID)
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	status := offlineScanStatusAccepted
	result := CheckinResponse{
		TicketID:    ticket.TicketID,
		EventID:     ticket.EventID,
		EmployeeID:  ticket.EmployeeID,
		Status:      offlineScanStatusAccepted,
		ReasonCode:  "accepted",
		ScannedAt:   scan.ScannedAt,
		Holder:      ticketHolderFromTicket(ticket),
		FamilyCount: ticket.FamilyCount,
	}
	if found {
		status = offlineScanStatusDuplicate
		result = duplicateOfflineResult(existing)
	} else if ticket.Status != TicketActive {
		status = offlineScanStatusConflict
		result.Status = offlineScanStatusConflict
		result.ReasonCode = "offline_conflict"
		result.ConflictReason = offlineConflictNotActive
	} else if !ticket.ExpiresAt.IsZero() && s.now().After(ticket.ExpiresAt) {
		status = offlineScanStatusConflict
		result.Status = offlineScanStatusConflict
		result.ReasonCode = "offline_conflict"
		result.ConflictReason = offlineConflictExpired
	} else {
		checkinID, err := newID("chk")
		if err != nil {
			return CheckinResponse{}, offlineScanStatusConflict, err
		}
		err = tx.QueryRow(ctx, `INSERT INTO checkin_records (checkin_id, ticket_id, staff_id, device_id, status, scanned_at)
			VALUES ($1,$2,$3,$4,'accepted',$5)
			ON CONFLICT (ticket_id) DO NOTHING
			RETURNING checkin_id`, checkinID, ticket.TicketID, actor.ID, req.DeviceID, scan.ScannedAt).Scan(&result.CheckinID)
		if errors.Is(err, pgx.ErrNoRows) {
			existing, found, err := s.findCheckinByTicketTx(ctx, tx, ticket.TicketID)
			if err != nil {
				return CheckinResponse{}, offlineScanStatusConflict, err
			}
			if !found {
				return CheckinResponse{}, offlineScanStatusConflict, conflict("ticket check-in winner was not found")
			}
			status = offlineScanStatusDuplicate
			result = duplicateOfflineResult(existing)
		} else if err != nil {
			return CheckinResponse{}, offlineScanStatusConflict, err
		} else if _, err := tx.Exec(ctx, `UPDATE tickets SET status = 'redeemed' WHERE ticket_id = $1`, ticket.TicketID); err != nil {
			return CheckinResponse{}, offlineScanStatusConflict, err
		} else if err := insertOfflineAcceptedSideEffectsTx(ctx, tx, actor, req, ticket); err != nil {
			return CheckinResponse{}, offlineScanStatusConflict, err
		}
	}
	if err := s.insertOfflineScanTx(ctx, tx, req, ticket.TicketID, status, scan.ScannedAt, tokenHash, result.ConflictReason); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	if status != offlineScanStatusAccepted {
		reason := result.ConflictReason
		if reason == "" {
			reason = offlineConflictRedeemed
		}
		if err := s.insertOfflineConflictAuditTx(ctx, tx, actor, req, "ticket", ticket.TicketID, ticket.EventID, status, reason); err != nil {
			return CheckinResponse{}, offlineScanStatusConflict, err
		}
	}
	return result, status, tx.Commit(ctx)
}

func (s *Service) recordOfflineUnknownConflict(ctx context.Context, actor Actor, req OfflineCheckinSyncRequest, scan OfflineCheckinScanInput, claims TicketClaims, reason string) (CheckinResponse, string, error) {
	result := conflictResultFromClaims(req, claims, scan.ScannedAt, reason)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	defer rollback(ctx, tx)
	if err := s.insertOfflineScanTx(ctx, tx, req, "", offlineScanStatusConflict, scan.ScannedAt, s.signer.HashToken(scan.SignedToken), reason); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	if err := s.insertOfflineConflictAuditTx(ctx, tx, actor, req, "offline_checkin_batch", req.BatchID, result.EventID, offlineScanStatusConflict, reason); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	return result, offlineScanStatusConflict, tx.Commit(ctx)
}

func (s *Service) recordKnownOfflineConflict(ctx context.Context, tx pgx.Tx, actor Actor, req OfflineCheckinSyncRequest, ticket Ticket, scannedAt time.Time, tokenHash string, reason string) (CheckinResponse, string, error) {
	result := CheckinResponse{
		TicketID:       ticket.TicketID,
		EventID:        ticket.EventID,
		EmployeeID:     ticket.EmployeeID,
		Status:         offlineScanStatusConflict,
		ReasonCode:     "offline_conflict",
		ScannedAt:      scannedAt,
		ConflictReason: reason,
		Holder:         ticketHolderFromTicket(ticket),
		FamilyCount:    ticket.FamilyCount,
	}
	if err := s.insertOfflineScanTx(ctx, tx, req, ticket.TicketID, offlineScanStatusConflict, scannedAt, tokenHash, reason); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	if err := s.insertOfflineConflictAuditTx(ctx, tx, actor, req, "ticket", ticket.TicketID, ticket.EventID, offlineScanStatusConflict, reason); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	return result, offlineScanStatusConflict, tx.Commit(ctx)
}

func (s *Service) insertOfflineScanTx(ctx context.Context, tx pgx.Tx, req OfflineCheckinSyncRequest, ticketID string, status string, scannedAt time.Time, tokenHash string, conflictReason string) error {
	scanID, err := newID("ofs")
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO offline_checkin_scans
		(scan_id, batch_id, ticket_id, device_id, status, token_hash, conflict_reason, scanned_at)
		VALUES ($1,$2,NULLIF($3, ''),$4,$5,$6,$7,$8)`, scanID, req.BatchID, ticketID, req.DeviceID, status, tokenHash, conflictReason, scannedAt)
	return err
}

func (s *Service) insertOfflineConflictAuditTx(ctx context.Context, tx pgx.Tx, actor Actor, req OfflineCheckinSyncRequest, entityType string, entityID string, eventID string, status string, reason string) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	metadata := map[string]interface{}{"event_id": eventID, "batch_id": req.BatchID, "device_id": req.DeviceID, "status": status, "reason": reason}
	if eventID != req.EventID {
		metadata["batch_event_id"] = req.EventID
	}
	return insertAudit(ctx, tx, auditID, actor, "offline_checkin.conflict", entityType, entityID, metadata)
}

func insertOfflineAcceptedSideEffectsTx(ctx context.Context, tx pgx.Tx, actor Actor, req OfflineCheckinSyncRequest, ticket Ticket) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, auditID, actor, "ticket.redeemed", "ticket", ticket.TicketID, map[string]interface{}{
		"event_id":  ticket.EventID,
		"batch_id":  req.BatchID,
		"device_id": req.DeviceID,
		"mode":      "offline_sync",
	}); err != nil {
		return err
	}
	return insertOutbox(ctx, tx, "ticket.redeemed", ticket.TicketID, map[string]interface{}{"ticket_id": ticket.TicketID, "event_id": ticket.EventID})
}

func conflictResultFromClaims(req OfflineCheckinSyncRequest, claims TicketClaims, scannedAt time.Time, reason string) CheckinResponse {
	eventID := claims.EventID
	if eventID == "" {
		eventID = req.EventID
	}
	return CheckinResponse{TicketID: claims.TicketID, EventID: eventID, EmployeeID: claims.EmployeeID, Status: offlineScanStatusConflict, ReasonCode: "offline_conflict", ScannedAt: scannedAt, ConflictReason: reason}
}

func claimsMatchTicket(claims TicketClaims, ticket Ticket) bool {
	return claims.TicketID == ticket.TicketID && claims.EventID == ticket.EventID && claims.EmployeeID == ticket.EmployeeID
}

func duplicateOfflineResult(existing CheckinResponse) CheckinResponse {
	existing.Duplicate = true
	existing.FirstScannedAt = existing.ScannedAt
	existing.ConflictReason = offlineConflictRedeemed
	existing.ReasonCode = "duplicate_scan"
	return existing
}
