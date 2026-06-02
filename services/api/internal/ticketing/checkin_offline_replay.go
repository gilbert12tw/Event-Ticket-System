package ticketing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Service) replayOfflineSync(ctx context.Context, req OfflineCheckinSyncRequest) (OfflineCheckinSyncResponse, error) {
	response := OfflineCheckinSyncResponse{BatchID: req.BatchID}
	rows, err := s.scanReplayOfflineRows(ctx, req)
	if err != nil {
		return OfflineCheckinSyncResponse{}, err
	}
	for _, scan := range req.Scans {
		if scan.ScannedAt.IsZero() {
			return OfflineCheckinSyncResponse{}, conflict("offline check-in batch cannot replay scans without scanned_at")
		}
		claims, _ := s.signer.Verify(scan.SignedToken)
		tokenHash := s.signer.HashToken(scan.SignedToken)
		row, found := rows[offlineReplayKey(tokenHash, scan.ScannedAt)]
		if !found {
			return OfflineCheckinSyncResponse{}, conflict("offline check-in batch cannot accept new scans after sync")
		}
		result, err := row.result(req, claims)
		if err != nil {
			return OfflineCheckinSyncResponse{}, err
		}
		response.Results = append(response.Results, result)
		incrementOfflineSyncCount(&response, row.status)
	}
	return response, nil
}

type offlineReplayRow struct {
	ticketID       string
	status         string
	conflictReason string
	scannedAt      time.Time
	checkin        CheckinResponse
	checkinFound   bool
	ticketFound    bool
	ticketEventID  string
	ticketEmployee string
	ticketHolder   TicketHolder
	ticketFamily   int
	firstScannedBy string
	firstScannedAt time.Time
}

func (s *Service) scanReplayOfflineRows(ctx context.Context, req OfflineCheckinSyncRequest) (map[string]offlineReplayRow, error) {
	tokenHashes := make([]string, 0, len(req.Scans))
	scannedAtValues := make([]time.Time, 0, len(req.Scans))
	for _, scan := range req.Scans {
		if scan.ScannedAt.IsZero() {
			continue
		}
		tokenHashes = append(tokenHashes, s.signer.HashToken(scan.SignedToken))
		scannedAtValues = append(scannedAtValues, scan.ScannedAt)
	}
	if len(tokenHashes) == 0 {
		return map[string]offlineReplayRow{}, nil
	}
	rows, err := s.db.Query(ctx, `WITH input AS (
			SELECT token_hash, scanned_at
			FROM unnest($2::text[], $3::timestamptz[]) AS i(token_hash, scanned_at)
		), matched AS (
			SELECT DISTINCT ON (i.token_hash, i.scanned_at)
				i.token_hash,
				i.scanned_at AS input_scanned_at,
				COALESCE(s.ticket_id, '') AS scan_ticket_id,
				s.status,
				s.conflict_reason,
				s.scanned_at AS scan_scanned_at
			FROM input i
			JOIN offline_checkin_scans s ON s.batch_id = $1
				AND s.token_hash = i.token_hash
				AND s.scanned_at = i.scanned_at
			ORDER BY i.token_hash, i.scanned_at, s.created_at ASC
		)
		SELECT m.token_hash,
			m.input_scanned_at,
			m.scan_ticket_id,
			m.status,
			m.conflict_reason,
			m.scan_scanned_at,
			COALESCE(c.checkin_id, ''),
			COALESCE(c.ticket_id, ''),
			COALESCE(ct.event_id, ''),
			COALESCE(cev.title, ''),
			COALESCE(ct.employee_id, ''),
			COALESCE(c.status, ''),
			COALESCE(c.scanned_at, 'epoch'::timestamptz),
			COALESCE(c.staff_id, ''),
			COALESCE(cr.family_count, 0),
			COALESCE(ce.full_name, ''),
			COALESCE(ce.department, ''),
			COALESCE(ce.site, ''),
			COALESCE(t.event_id, ''),
			COALESCE(t.employee_id, ''),
			COALESCE(tr.family_count, 0),
			COALESCE(te.full_name, ''),
			COALESCE(te.department, ''),
			COALESCE(te.site, '')
		FROM matched m
		LEFT JOIN checkin_records c ON c.ticket_id = m.scan_ticket_id
		LEFT JOIN tickets ct ON ct.ticket_id = c.ticket_id
		LEFT JOIN events cev ON cev.event_id = ct.event_id
		LEFT JOIN registrations cr ON cr.registration_id = ct.registration_id
		LEFT JOIN employees ce ON ce.employee_id = ct.employee_id
		LEFT JOIN tickets t ON t.ticket_id = m.scan_ticket_id
		LEFT JOIN registrations tr ON tr.registration_id = t.registration_id
		LEFT JOIN employees te ON te.employee_id = t.employee_id`, req.BatchID, tokenHashes, scannedAtValues)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	replayRows := make(map[string]offlineReplayRow, len(tokenHashes))
	for rows.Next() {
		var tokenHash string
		var inputScannedAt time.Time
		var row offlineReplayRow
		var checkinID, checkinTicketID, checkinEventID, checkinEventTitle, checkinEmployeeID, checkinStatus string
		var checkinScannedAt time.Time
		var checkinFamilyCount int
		var checkinHolder TicketHolder
		var ticketEventID, ticketEmployeeID string
		var ticketFamilyCount int
		var ticketHolder TicketHolder
		if err := rows.Scan(
			&tokenHash,
			&inputScannedAt,
			&row.ticketID,
			&row.status,
			&row.conflictReason,
			&row.scannedAt,
			&checkinID,
			&checkinTicketID,
			&checkinEventID,
			&checkinEventTitle,
			&checkinEmployeeID,
			&checkinStatus,
			&checkinScannedAt,
			&row.firstScannedBy,
			&checkinFamilyCount,
			&checkinHolder.DisplayName,
			&checkinHolder.Department,
			&checkinHolder.City,
			&ticketEventID,
			&ticketEmployeeID,
			&ticketFamilyCount,
			&ticketHolder.DisplayName,
			&ticketHolder.Department,
			&ticketHolder.City,
		); err != nil {
			return nil, err
		}
		if checkinID != "" {
			row.checkinFound = true
			row.firstScannedAt = checkinScannedAt
			row.checkin = CheckinResponse{
				CheckinID:   checkinID,
				TicketID:    checkinTicketID,
				EventID:     checkinEventID,
				EventTitle:  checkinEventTitle,
				EmployeeID:  checkinEmployeeID,
				Status:      checkinStatus,
				ScannedAt:   checkinScannedAt,
				Holder:      checkinHolder,
				FamilyCount: checkinFamilyCount,
			}
		}
		if ticketEventID != "" {
			row.ticketFound = true
			row.ticketEventID = ticketEventID
			row.ticketEmployee = ticketEmployeeID
			row.ticketHolder = ticketHolder
			row.ticketFamily = ticketFamilyCount
		}
		replayRows[offlineReplayKey(tokenHash, inputScannedAt)] = row
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return replayRows, nil
}

func offlineReplayKey(tokenHash string, scannedAt time.Time) string {
	return tokenHash + "|" + scannedAt.UTC().Format(time.RFC3339Nano)
}

func (row offlineReplayRow) result(req OfflineCheckinSyncRequest, claims TicketClaims) (CheckinResponse, error) {
	if row.ticketID == "" {
		return conflictResultFromClaims(req, claims, row.scannedAt, row.conflictReason), nil
	}
	if row.status == offlineScanStatusAccepted {
		if !row.checkinFound {
			return CheckinResponse{}, conflict("offline check-in accepted scan is missing check-in record")
		}
		row.checkin.ReasonCode = "accepted"
		row.checkin.Duplicate = false
		return row.checkin, nil
	}
	if row.status == offlineScanStatusDuplicate {
		if !row.checkinFound {
			return CheckinResponse{}, conflict("offline check-in duplicate scan is missing first check-in record")
		}
		duplicate := duplicateOfflineResult(row.checkin)
		duplicate.FirstScannedBy = row.firstScannedBy
		duplicate.FirstScannedAt = row.firstScannedAt
		return duplicate, nil
	}
	if !row.ticketFound {
		return CheckinResponse{}, fmt.Errorf("offline check-in conflict scan ticket snapshot is missing")
	}
	return CheckinResponse{
		TicketID:       row.ticketID,
		EventID:        row.ticketEventID,
		EmployeeID:     row.ticketEmployee,
		Status:         offlineScanStatusConflict,
		ReasonCode:     "offline_conflict",
		ScannedAt:      row.scannedAt,
		ConflictReason: row.conflictReason,
		Holder:         row.ticketHolder,
		FamilyCount:    row.ticketFamily,
	}, nil
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
