package ticketing

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Service) replayOfflineSync(ctx context.Context, req OfflineCheckinSyncRequest) (OfflineCheckinSyncResponse, error) {
	response := OfflineCheckinSyncResponse{BatchID: req.BatchID}
	for _, scan := range req.Scans {
		if scan.ScannedAt.IsZero() {
			return OfflineCheckinSyncResponse{}, conflict("offline check-in batch cannot replay scans without scanned_at")
		}
		claims, _ := s.signer.Verify(scan.SignedToken)
		tokenHash := s.signer.HashToken(scan.SignedToken)
		result, status, found, err := s.replayOfflineScan(ctx, req, scan, claims, tokenHash)
		if err != nil {
			return OfflineCheckinSyncResponse{}, err
		}
		if !found {
			return OfflineCheckinSyncResponse{}, conflict("offline check-in batch cannot accept new scans after sync")
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
	return response, nil
}

func (s *Service) replayOfflineScan(ctx context.Context, req OfflineCheckinSyncRequest, scan OfflineCheckinScanInput, claims TicketClaims, tokenHash string) (CheckinResponse, string, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, false, err
	}
	defer rollback(ctx, tx)

	var ticketID, status, conflictReason string
	var scannedAt time.Time
	err = tx.QueryRow(ctx, `SELECT COALESCE(ticket_id, ''), status, conflict_reason, scanned_at
		FROM offline_checkin_scans
		WHERE batch_id = $1 AND token_hash = $2 AND scanned_at = $3
		ORDER BY created_at ASC
		LIMIT 1`, req.BatchID, tokenHash, scan.ScannedAt).
		Scan(&ticketID, &status, &conflictReason, &scannedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CheckinResponse{}, offlineScanStatusConflict, false, nil
	}
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, false, err
	}

	result, err := s.replayedOfflineResultTx(ctx, tx, replayedOfflineResultInput{
		req:            req,
		ticketID:       ticketID,
		status:         status,
		conflictReason: conflictReason,
		scannedAt:      scannedAt,
		claims:         claims,
	})
	if err != nil {
		return CheckinResponse{}, offlineScanStatusConflict, false, err
	}
	return result, status, true, tx.Commit(ctx)
}

type replayedOfflineResultInput struct {
	req            OfflineCheckinSyncRequest
	ticketID       string
	status         string
	conflictReason string
	scannedAt      time.Time
	claims         TicketClaims
}

func (s *Service) replayedOfflineResultTx(ctx context.Context, tx pgx.Tx, input replayedOfflineResultInput) (CheckinResponse, error) {
	if input.ticketID == "" {
		return conflictResultFromClaims(input.req, input.claims, input.scannedAt, input.conflictReason), nil
	}

	existing, found, err := s.findCheckinByTicketTx(ctx, tx, input.ticketID)
	if err != nil {
		return CheckinResponse{}, err
	}
	if input.status == offlineScanStatusAccepted {
		if !found {
			return CheckinResponse{}, conflict("offline check-in accepted scan is missing check-in record")
		}
		existing.ReasonCode = "accepted"
		existing.Duplicate = false
		return existing, nil
	}
	if input.status == offlineScanStatusDuplicate {
		if !found {
			return CheckinResponse{}, conflict("offline check-in duplicate scan is missing first check-in record")
		}
		return duplicateOfflineResult(existing), nil
	}

	ticket, err := ticketSnapshotTx(ctx, tx, input.ticketID)
	if err != nil {
		return CheckinResponse{}, err
	}
	return CheckinResponse{
		TicketID:       ticket.TicketID,
		EventID:        ticket.EventID,
		EmployeeID:     ticket.EmployeeID,
		Status:         offlineScanStatusConflict,
		ReasonCode:     "offline_conflict",
		ScannedAt:      input.scannedAt,
		ConflictReason: input.conflictReason,
		Holder:         ticketHolderFromTicket(ticket),
		FamilyCount:    ticket.FamilyCount,
	}, nil
}
