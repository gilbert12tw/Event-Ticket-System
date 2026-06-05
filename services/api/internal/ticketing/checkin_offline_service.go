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
	offlineConflictNotStarted     = checkinNotStartedReason
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
	req, err := normalizeOfflineCheckinSyncRequest(req)
	if err != nil {
		return OfflineCheckinSyncResponse{}, err
	}
	batchLockTx, err := s.db.Begin(ctx)
	if err != nil {
		return OfflineCheckinSyncResponse{}, err
	}
	defer rollback(ctx, batchLockTx)

	batchStatus, err := s.validateOfflineBatchTx(ctx, batchLockTx, actor, req)
	if err != nil {
		return OfflineCheckinSyncResponse{}, err
	}
	if batchStatus != "open" {
		if err := batchLockTx.Commit(ctx); err != nil {
			return OfflineCheckinSyncResponse{}, err
		}
		return s.replayOfflineSync(ctx, req)
	}
	response, err := s.syncOpenOfflineBatch(ctx, actor, req)
	if err != nil {
		return OfflineCheckinSyncResponse{}, err
	}
	if err := finishOfflineBatchSyncTx(ctx, batchLockTx, response); err != nil {
		return OfflineCheckinSyncResponse{}, err
	}
	return response, batchLockTx.Commit(ctx)
}

func (s *Service) validateOfflineBatchTx(ctx context.Context, tx pgx.Tx, actor Actor, req OfflineCheckinSyncRequest) (string, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`, req.BatchID); err != nil {
		return "", err
	}
	var eventID, deviceID, staffID, status string
	var validUntil time.Time
	var packageSignature string
	err := tx.QueryRow(ctx, `SELECT event_id, device_id, staff_id, status, valid_until, package_signature
		FROM offline_checkin_batches WHERE batch_id = $1`, req.BatchID).
		Scan(&eventID, &deviceID, &staffID, &status, &validUntil, &packageSignature)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", notFound("offline check-in batch not found")
	}
	if err != nil {
		return "", err
	}
	if staffID != actor.ID {
		return "", forbidden("offline check-in batch belongs to a different staff member")
	}
	if eventID != req.EventID {
		return "", conflict("offline check-in batch event mismatch")
	}
	if deviceID != req.DeviceID {
		return "", conflict("offline check-in batch device mismatch")
	}
	if packageSignature != req.PackageSignature {
		return "", badRequest("offline package signature does not match batch")
	}
	claims, err := s.signer.VerifyOfflinePackage(req.PackageSignature)
	if err != nil {
		return "", badRequest("invalid offline package signature")
	}
	if claims.BatchID != req.BatchID || claims.EventID != req.EventID || claims.DeviceID != req.DeviceID || claims.StaffID != actor.ID {
		return "", badRequest("offline package claims do not match request")
	}
	if !claims.ValidUntil.Equal(validUntil.UTC()) {
		return "", badRequest("offline package expiry does not match batch")
	}
	if status != "open" && status != "synced" && status != "conflict" {
		return "", conflict("offline check-in batch has invalid status")
	}
	if status == "open" && !validUntil.After(s.now()) {
		return "", conflict("offline package is expired")
	}
	return status, nil
}

func (s *Service) syncOfflineScan(ctx context.Context, actor Actor, req OfflineCheckinSyncRequest, scan OfflineCheckinScanInput) (CheckinResponse, string, error) {
	if scan.ScannedAt.IsZero() {
		scan.ScannedAt = s.now()
	}
	tokenHash := s.signer.HashToken(scan.SignedToken)
	claims, err := s.signer.Verify(scan.SignedToken)
	if err != nil {
		return s.syncInvalidOfflineToken(ctx, actor, req, scan, tokenHash)
	}
	if result, status, found, err := s.replayOfflineScan(ctx, req, scan, claims, tokenHash); err != nil || found {
		return result, status, err
	}
	return s.recordVerifiedOfflineScan(ctx, actor, req, scan, claims, tokenHash)
}

func (s *Service) syncInvalidOfflineToken(ctx context.Context, actor Actor, req OfflineCheckinSyncRequest, scan OfflineCheckinScanInput, tokenHash string) (CheckinResponse, string, error) {
	if result, status, found, replayErr := s.replayOfflineScan(ctx, req, scan, TicketClaims{}, tokenHash); replayErr != nil || found {
		return result, status, replayErr
	}
	return s.recordOfflineUnknownConflict(ctx, actor, req, scan, TicketClaims{}, offlineConflictInvalidToken)
}

func (s *Service) recordVerifiedOfflineScan(ctx context.Context, actor Actor, req OfflineCheckinSyncRequest, scan OfflineCheckinScanInput, claims TicketClaims, tokenHash string) (CheckinResponse, string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	defer rollback(ctx, tx)

	ticket, found, err := scanOfflineTicketByHashTx(ctx, tx, tokenHash)
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	if !found {
		return s.recordMissingOfflineTicketTx(ctx, tx, actor, req, scan, claims, tokenHash)
	}
	if !claimsMatchTicket(claims, ticket) {
		return s.recordKnownOfflineConflict(ctx, tx, actor, newKnownOfflineConflict(req, ticket, scan.ScannedAt, tokenHash, offlineConflictClaimsMismatch))
	}
	if ticket.EventID != req.EventID {
		return s.recordKnownOfflineConflict(ctx, tx, actor, newKnownOfflineConflict(req, ticket, scan.ScannedAt, tokenHash, offlineConflictEventMismatch))
	}
	result, status, err := s.applyOfflineTicketScanTx(ctx, tx, actor, req, scan, ticket)
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	if err := s.insertRecordedOfflineScanEffectsTx(ctx, tx, actor, offlineRecordedScanEffects{
		req:       req,
		scan:      scan,
		tokenHash: tokenHash,
		ticket:    ticket,
		result:    result,
		status:    status,
	}); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	return result, status, tx.Commit(ctx)
}

func scanOfflineTicketByHashTx(ctx context.Context, tx pgx.Tx, tokenHash string) (Ticket, bool, error) {
	return scanLockedTicketByHashTx(ctx, tx, tokenHash)
}

func scanLockedTicketByHashTx(ctx context.Context, tx pgx.Tx, tokenHash string) (Ticket, bool, error) {
	var ticket Ticket
	err := scanCheckinTicketRow(tx.QueryRow(ctx, `SELECT `+checkinTicketSelectColumns+`
		FROM tickets t
		JOIN registrations r ON r.registration_id = t.registration_id
		JOIN events ev ON ev.event_id = t.event_id
		JOIN employees e ON e.employee_id = t.employee_id
		WHERE t.signed_token_hash = $1 FOR UPDATE OF t`, tokenHash), &ticket)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, false, nil
	}
	return ticket, err == nil, err
}

func (s *Service) recordMissingOfflineTicketTx(ctx context.Context, tx pgx.Tx, actor Actor, req OfflineCheckinSyncRequest, scan OfflineCheckinScanInput, claims TicketClaims, tokenHash string) (CheckinResponse, string, error) {
	result := conflictResultFromClaims(req, claims, scan.ScannedAt, offlineConflictNotFound)
	if err := s.insertOfflineScanTx(ctx, tx, newOfflineScanRecord(req, "", offlineScanStatusConflict, scan.ScannedAt, tokenHash, result.ConflictReason)); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	if err := s.insertOfflineConflictAuditTx(ctx, tx, actor, newOfflineConflictAudit(req, "offline_checkin_batch", req.BatchID, result.EventID, offlineScanStatusConflict, result.ConflictReason)); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	return result, offlineScanStatusConflict, tx.Commit(ctx)
}

func (s *Service) applyOfflineTicketScanTx(ctx context.Context, tx pgx.Tx, actor Actor, req OfflineCheckinSyncRequest, scan OfflineCheckinScanInput, ticket Ticket) (CheckinResponse, string, error) {
	existing, found, err := s.findCheckinByTicketTx(ctx, tx, ticket.TicketID)
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	result := CheckinResponse{
		TicketID:    ticket.TicketID,
		EventID:     ticket.EventID,
		EventTitle:  ticket.EventTitle,
		EmployeeID:  ticket.EmployeeID,
		Status:      offlineScanStatusAccepted,
		ReasonCode:  "accepted",
		ScannedAt:   scan.ScannedAt,
		Holder:      ticketHolderFromTicket(ticket),
		FamilyCount: ticket.FamilyCount,
	}
	if found {
		return duplicateOfflineResult(existing), offlineScanStatusDuplicate, nil
	} else if ticket.Status != TicketActive {
		result.Status = offlineScanStatusConflict
		result.ReasonCode = "offline_conflict"
		result.ConflictReason = offlineConflictNotActive
		return result, offlineScanStatusConflict, nil
	} else if !isCheckinAfterEventStart(ticket.EventStartsAt, scan.ScannedAt) {
		result.Status = offlineScanStatusConflict
		result.ReasonCode = "offline_conflict"
		result.ConflictReason = offlineConflictNotStarted
		return result, offlineScanStatusConflict, nil
	} else if !ticket.ExpiresAt.IsZero() && s.now().After(ticket.ExpiresAt) {
		result.Status = offlineScanStatusConflict
		result.ReasonCode = "offline_conflict"
		result.ConflictReason = offlineConflictExpired
		return result, offlineScanStatusConflict, nil
	}
	return s.acceptOfflineTicketScanTx(ctx, tx, actor, req, scan, ticket, result)
}

func (s *Service) acceptOfflineTicketScanTx(ctx context.Context, tx pgx.Tx, actor Actor, req OfflineCheckinSyncRequest, scan OfflineCheckinScanInput, ticket Ticket, result CheckinResponse) (CheckinResponse, string, error) {
	checkinID, err := newID("chk")
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO checkin_records (checkin_id, ticket_id, staff_id, device_id, status, scanned_at)
		VALUES ($1,$2,$3,$4,'accepted',$5)
		ON CONFLICT (ticket_id) DO NOTHING
		RETURNING checkin_id`, checkinID, ticket.TicketID, actor.ID, req.DeviceID, scan.ScannedAt).Scan(&result.CheckinID)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.duplicateOfflineWinnerResultTx(ctx, tx, ticket.TicketID)
	}
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	if _, err := tx.Exec(ctx, `UPDATE tickets SET status = 'redeemed' WHERE ticket_id = $1`, ticket.TicketID); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	if err := insertOfflineAcceptedSideEffectsTx(ctx, tx, actor, req, ticket); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	return result, offlineScanStatusAccepted, nil
}

func (s *Service) duplicateOfflineWinnerResultTx(ctx context.Context, tx pgx.Tx, ticketID string) (CheckinResponse, string, error) {
	existing, found, err := s.findCheckinByTicketTx(ctx, tx, ticketID)
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	if !found {
		return CheckinResponse{}, offlineScanStatusConflict, conflict("ticket check-in winner was not found")
	}
	return duplicateOfflineResult(existing), offlineScanStatusDuplicate, nil
}

type offlineRecordedScanEffects struct {
	req       OfflineCheckinSyncRequest
	scan      OfflineCheckinScanInput
	tokenHash string
	ticket    Ticket
	result    CheckinResponse
	status    string
}

func (s *Service) insertRecordedOfflineScanEffectsTx(ctx context.Context, tx pgx.Tx, actor Actor, effects offlineRecordedScanEffects) error {
	if err := s.insertOfflineScanTx(ctx, tx, newOfflineScanRecord(effects.req, effects.ticket.TicketID, effects.status, effects.scan.ScannedAt, effects.tokenHash, effects.result.ConflictReason)); err != nil {
		return err
	}
	if effects.status != offlineScanStatusAccepted {
		reason := effects.result.ConflictReason
		if reason == "" {
			reason = offlineConflictRedeemed
		}
		if err := s.insertOfflineConflictAuditTx(ctx, tx, actor, newOfflineConflictAudit(effects.req, "ticket", effects.ticket.TicketID, effects.ticket.EventID, effects.status, reason)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) recordOfflineUnknownConflict(ctx context.Context, actor Actor, req OfflineCheckinSyncRequest, scan OfflineCheckinScanInput, claims TicketClaims, reason string) (CheckinResponse, string, error) {
	result := conflictResultFromClaims(req, claims, scan.ScannedAt, reason)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	defer rollback(ctx, tx)
	if err := s.insertOfflineScanTx(ctx, tx, newOfflineScanRecord(req, "", offlineScanStatusConflict, scan.ScannedAt, s.signer.HashToken(scan.SignedToken), reason)); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	if err := s.insertOfflineConflictAuditTx(ctx, tx, actor, newOfflineConflictAudit(req, "offline_checkin_batch", req.BatchID, result.EventID, offlineScanStatusConflict, reason)); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	return result, offlineScanStatusConflict, tx.Commit(ctx)
}

type knownOfflineConflict struct {
	req       OfflineCheckinSyncRequest
	ticket    Ticket
	scannedAt time.Time
	tokenHash string
	reason    string
}

func newKnownOfflineConflict(req OfflineCheckinSyncRequest, ticket Ticket, scannedAt time.Time, tokenHash string, reason string) knownOfflineConflict {
	return knownOfflineConflict{req: req, ticket: ticket, scannedAt: scannedAt, tokenHash: tokenHash, reason: reason}
}

func (s *Service) recordKnownOfflineConflict(ctx context.Context, tx pgx.Tx, actor Actor, conflict knownOfflineConflict) (CheckinResponse, string, error) {
	result := CheckinResponse{
		TicketID:       conflict.ticket.TicketID,
		EventID:        conflict.ticket.EventID,
		EventTitle:     conflict.ticket.EventTitle,
		EmployeeID:     conflict.ticket.EmployeeID,
		Status:         offlineScanStatusConflict,
		ReasonCode:     "offline_conflict",
		ScannedAt:      conflict.scannedAt,
		ConflictReason: conflict.reason,
		Holder:         ticketHolderFromTicket(conflict.ticket),
		FamilyCount:    conflict.ticket.FamilyCount,
	}
	if err := s.insertOfflineScanTx(ctx, tx, newOfflineScanRecord(conflict.req, conflict.ticket.TicketID, offlineScanStatusConflict, conflict.scannedAt, conflict.tokenHash, conflict.reason)); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	if err := s.insertOfflineConflictAuditTx(ctx, tx, actor, newOfflineConflictAudit(conflict.req, "ticket", conflict.ticket.TicketID, conflict.ticket.EventID, offlineScanStatusConflict, conflict.reason)); err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, err
	}
	return result, offlineScanStatusConflict, tx.Commit(ctx)
}

type offlineScanRecord struct {
	req            OfflineCheckinSyncRequest
	ticketID       string
	status         string
	scannedAt      time.Time
	tokenHash      string
	conflictReason string
}

func newOfflineScanRecord(req OfflineCheckinSyncRequest, ticketID string, status string, scannedAt time.Time, tokenHash string, conflictReason string) offlineScanRecord {
	return offlineScanRecord{req: req, ticketID: ticketID, status: status, scannedAt: scannedAt, tokenHash: tokenHash, conflictReason: conflictReason}
}

func (s *Service) insertOfflineScanTx(ctx context.Context, tx pgx.Tx, scan offlineScanRecord) error {
	scanID, err := newID("ofs")
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO offline_checkin_scans
		(scan_id, batch_id, ticket_id, device_id, status, token_hash, conflict_reason, scanned_at)
		VALUES ($1,$2,NULLIF($3, ''),$4,$5,$6,$7,$8)`,
		scanID, scan.req.BatchID, scan.ticketID, scan.req.DeviceID, scan.status, scan.tokenHash, scan.conflictReason, scan.scannedAt)
	return err
}

type offlineConflictAudit struct {
	req        OfflineCheckinSyncRequest
	entityType string
	entityID   string
	eventID    string
	status     string
	reason     string
}

func newOfflineConflictAudit(req OfflineCheckinSyncRequest, entityType string, entityID string, eventID string, status string, reason string) offlineConflictAudit {
	return offlineConflictAudit{req: req, entityType: entityType, entityID: entityID, eventID: eventID, status: status, reason: reason}
}

func (s *Service) insertOfflineConflictAuditTx(ctx context.Context, tx pgx.Tx, actor Actor, audit offlineConflictAudit) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	metadata := map[string]interface{}{"event_id": audit.eventID, "batch_id": audit.req.BatchID, "device_id": audit.req.DeviceID, "status": audit.status, "reason": audit.reason}
	if audit.eventID != audit.req.EventID {
		metadata["batch_event_id"] = audit.req.EventID
	}
	return insertAudit(ctx, tx, newAuditRecord(auditID, actor, "offline_checkin.conflict", audit.entityType, audit.entityID, metadata))
}

func insertOfflineAcceptedSideEffectsTx(ctx context.Context, tx pgx.Tx, actor Actor, req OfflineCheckinSyncRequest, ticket Ticket) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor, ticketRedeemedEventType, "ticket", ticket.TicketID, map[string]interface{}{
		"event_id":  ticket.EventID,
		"batch_id":  req.BatchID,
		"device_id": req.DeviceID,
		"mode":      "offline_sync",
	})); err != nil {
		return err
	}
	return insertOutbox(ctx, tx, ticketRedeemedEventType, ticket.TicketID, ticketOutboxPayload(ticket, map[string]interface{}{
		"batch_id":  req.BatchID,
		"device_id": req.DeviceID,
		"mode":      "offline_sync",
	}))
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
