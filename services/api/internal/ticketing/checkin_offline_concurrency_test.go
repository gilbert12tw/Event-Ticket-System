package ticketing

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncOfflineCheckinsConcurrentSameBatchReplaysFirstResult(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, ticket := createOfflineSyncTicket(t, service, ctx, "Concurrent Offline Replay", "E1001", "offline-concurrent-ticket")
	pkg, err := service.OfflineCheckinPackage(ctx, staff, event.EventID, "gate-1")
	require.NoError(t, err)
	scannedAt := event.StartsAt
	req := OfflineCheckinSyncRequest{
		BatchID:          pkg.BatchID,
		EventID:          event.EventID,
		DeviceID:         "gate-1",
		PackageSignature: pkg.PackageSignature,
		Scans:            []OfflineCheckinScanInput{{SignedToken: ticket.SignedToken, ScannedAt: scannedAt}},
	}

	lockTx, err := service.db.Begin(ctx)
	require.NoError(t, err)
	defer rollback(ctx, lockTx)
	_, err = lockTx.Exec(ctx, `SELECT ticket_id FROM tickets WHERE ticket_id = $1 FOR UPDATE`, ticket.TicketID)
	require.NoError(t, err)

	type syncResult struct {
		response OfflineCheckinSyncResponse
		err      error
	}
	results := make(chan syncResult, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response, err := service.SyncOfflineCheckins(ctx, staff, req)
			results <- syncResult{response: response, err: err}
		}()
	}
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, lockTx.Commit(ctx))
	wg.Wait()
	close(results)

	var responses []OfflineCheckinSyncResponse
	for result := range results {
		require.NoError(t, result.err)
		responses = append(responses, result.response)
	}
	require.Len(t, responses, 2)
	for _, response := range responses {
		assert.Equal(t, 1, response.Accepted)
		assert.Zero(t, response.Duplicate)
		assert.Zero(t, response.Conflict)
		require.Len(t, response.Results, 1)
		assert.Equal(t, offlineScanStatusAccepted, response.Results[0].Status)
		assert.Equal(t, ticket.TicketID, response.Results[0].TicketID)
	}
	assert.Equal(t, responses[0].Results[0].CheckinID, responses[1].Results[0].CheckinID)

	tokenHash := service.signer.HashToken(ticket.SignedToken)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1 AND token_hash = $2 AND scanned_at = $3`, []interface{}{pkg.BatchID, tokenHash, scannedAt}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'ticket.redeemed' AND entity_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'ticket.redeemed' AND aggregate_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'offline_checkin.conflict' AND metadata->>'batch_id' = $1`, []interface{}{pkg.BatchID}, 0)
}
