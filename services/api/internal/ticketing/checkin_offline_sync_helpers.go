package ticketing

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func normalizeOfflineCheckinSyncRequest(req OfflineCheckinSyncRequest) (OfflineCheckinSyncRequest, error) {
	req.BatchID = strings.TrimSpace(req.BatchID)
	req.EventID = strings.TrimSpace(req.EventID)
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	req.PackageSignature = strings.TrimSpace(req.PackageSignature)
	switch {
	case req.BatchID == "":
		return OfflineCheckinSyncRequest{}, badRequest("batch_id is required")
	case req.EventID == "":
		return OfflineCheckinSyncRequest{}, badRequest("event_id is required")
	case req.DeviceID == "":
		return OfflineCheckinSyncRequest{}, badRequest("device_id is required")
	case req.PackageSignature == "":
		return OfflineCheckinSyncRequest{}, badRequest("package_signature is required")
	}
	for i, scan := range req.Scans {
		if scan.ScannedAt.IsZero() {
			return OfflineCheckinSyncRequest{}, badRequest("scans[" + strconv.Itoa(i) + "].scanned_at is required")
		}
	}
	return req, nil
}

func (s *Service) syncOpenOfflineBatch(ctx context.Context, actor Actor, req OfflineCheckinSyncRequest) (OfflineCheckinSyncResponse, error) {
	response := OfflineCheckinSyncResponse{BatchID: req.BatchID}
	for _, scan := range req.Scans {
		result, status, err := s.syncOfflineScan(ctx, actor, req, scan)
		if err != nil {
			return OfflineCheckinSyncResponse{}, err
		}
		response.Results = append(response.Results, result)
		incrementOfflineSyncCount(&response, status)
	}
	return response, nil
}

func incrementOfflineSyncCount(response *OfflineCheckinSyncResponse, status string) {
	switch status {
	case offlineScanStatusAccepted:
		response.Accepted++
	case offlineScanStatusDuplicate:
		response.Duplicate++
	default:
		response.Conflict++
	}
}

func finishOfflineBatchSyncTx(ctx context.Context, tx pgx.Tx, response OfflineCheckinSyncResponse) error {
	_, err := tx.Exec(ctx, `UPDATE offline_checkin_batches SET status = CASE WHEN $2 > 0 THEN 'conflict' ELSE 'synced' END, synced_at = now() WHERE batch_id = $1`, response.BatchID, response.Conflict)
	return err
}
