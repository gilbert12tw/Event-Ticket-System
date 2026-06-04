package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncOfflineCheckinsValidatesBatchOwnershipBeforeScans(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, ticket := createOfflineSyncTicket(t, service, ctx, "Ownership Check", "E1001", "ownership-ticket")
	otherEvent, _ := createOfflineSyncTicket(t, service, ctx, "Other Ownership Check", "E1002", "ownership-other")

	tests := []struct {
		name      string
		actor     Actor
		eventID   string
		deviceID  string
		wantError int
	}{
		{name: "staff", actor: Actor{ID: "staff-2", Role: RoleCheckinStaff}, eventID: event.EventID, deviceID: "gate-1", wantError: 403},
		{name: "event", actor: staff, eventID: otherEvent.EventID, deviceID: "gate-1", wantError: 409},
		{name: "device", actor: staff, eventID: event.EventID, deviceID: "gate-2", wantError: 409},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, err := service.OfflineCheckinPackage(ctx, staff, event.EventID, "gate-1")
			require.NoError(t, err)
			_, err = service.SyncOfflineCheckins(ctx, tt.actor, OfflineCheckinSyncRequest{
				BatchID:          pkg.BatchID,
				EventID:          tt.eventID,
				DeviceID:         tt.deviceID,
				PackageSignature: pkg.PackageSignature,
				Scans:            []OfflineCheckinScanInput{{SignedToken: ticket.SignedToken, ScannedAt: time.Now().UTC()}},
			})
			require.Error(t, err, "expected ownership error")
			assert.Equal(t, tt.wantError, ErrorStatus(err))
			assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{ticket.TicketID}, 0)
			assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1`, []interface{}{pkg.BatchID}, 0)
			assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'offline_checkin.conflict' AND metadata->>'batch_id' = $1`, []interface{}{pkg.BatchID}, 0)
		})
	}
}

func TestSyncOfflineCheckinsRejectsMissingScannedAtBeforeSideEffects(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, ticket := createOfflineSyncTicket(t, service, ctx, "Missing Scanned At", "E1001", "missing-scanned-at-ticket")
	pkg, err := service.OfflineCheckinPackage(ctx, staff, event.EventID, "gate-1")
	require.NoError(t, err)

	_, err = service.SyncOfflineCheckins(ctx, staff, OfflineCheckinSyncRequest{
		BatchID:          pkg.BatchID,
		EventID:          event.EventID,
		DeviceID:         "gate-1",
		PackageSignature: pkg.PackageSignature,
		Scans:            []OfflineCheckinScanInput{{SignedToken: ticket.SignedToken}},
	})

	require.Error(t, err)
	assert.Equal(t, 400, ErrorStatus(err))
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{ticket.TicketID}, 0)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1`, []interface{}{pkg.BatchID}, 0)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE metadata->>'batch_id' = $1`, []interface{}{pkg.BatchID}, 0)
	var batchStatus string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT status FROM offline_checkin_batches WHERE batch_id = $1`, pkg.BatchID).Scan(&batchStatus))
	assert.Equal(t, "open", batchStatus)
}

func TestSyncOfflineCheckinsPreservesPerScanConflictsAndAudits(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, ticket := createOfflineSyncTicket(t, service, ctx, "Per Scan Conflicts", "E1001", "conflict-ticket")
	_, otherTicket := createOfflineSyncTicket(t, service, ctx, "Mismatched Event Token", "E1002", "mismatch-ticket")
	_, claimsMismatchTicket := createOfflineSyncTicket(t, service, ctx, "Mismatched Claims Token", "E1001", "claims-mismatch-ticket")
	claimsMismatchToken, err := service.signer.Sign(TicketClaims{TicketID: claimsMismatchTicket.TicketID, EventID: claimsMismatchTicket.EventID, EmployeeID: "E1002"})
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `UPDATE tickets SET signed_token_hash = $1 WHERE ticket_id = $2`, service.signer.HashToken(claimsMismatchToken), claimsMismatchTicket.TicketID)
	require.NoError(t, err)
	pkg, err := service.OfflineCheckinPackage(ctx, staff, event.EventID, "gate-1")
	require.NoError(t, err)
	require.Len(t, pkg.Tickets, 1)
	assert.NotEmpty(t, pkg.Tickets[0].Holder.DisplayName)
	assert.NotEmpty(t, pkg.Tickets[0].Holder.Department)
	assert.NotEmpty(t, pkg.Tickets[0].Holder.City)
	scannedAt := time.Date(2026, 6, 2, 10, 12, 13, 123456789, time.UTC)
	scans := []OfflineCheckinScanInput{
		{SignedToken: "bad.token", ScannedAt: scannedAt.Add(time.Second)},
		{SignedToken: otherTicket.SignedToken, ScannedAt: scannedAt.Add(2 * time.Second)},
		{SignedToken: claimsMismatchToken, ScannedAt: scannedAt.Add(3 * time.Second)},
		{SignedToken: ticket.SignedToken, ScannedAt: scannedAt.Add(4 * time.Second)},
	}

	response, err := service.SyncOfflineCheckins(ctx, staff, OfflineCheckinSyncRequest{
		BatchID:          pkg.BatchID,
		EventID:          event.EventID,
		DeviceID:         "gate-1",
		PackageSignature: pkg.PackageSignature,
		Scans:            scans,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, response.Accepted)
	assert.Equal(t, 0, response.Duplicate)
	assert.Equal(t, 3, response.Conflict)
	require.Len(t, response.Results, 4)
	assert.Equal(t, "conflict", response.Results[0].Status)
	assert.Equal(t, "invalid_ticket_token", response.Results[0].ConflictReason)
	assert.Equal(t, "conflict", response.Results[1].Status)
	assert.Equal(t, "offline_scan_event_mismatch", response.Results[1].ConflictReason)
	assert.Equal(t, "conflict", response.Results[2].Status)
	assert.Equal(t, "ticket_token_claims_mismatch", response.Results[2].ConflictReason)
	assert.Equal(t, "accepted", response.Results[3].Status)
	assert.Equal(t, ticket.TicketID, response.Results[3].TicketID)

	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{otherTicket.TicketID}, 0)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{claimsMismatchTicket.TicketID}, 0)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1 AND status = 'accepted'`, []interface{}{pkg.BatchID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1 AND status = 'conflict'`, []interface{}{pkg.BatchID}, 3)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1 AND status = 'conflict' AND ticket_id IS NULL`, []interface{}{pkg.BatchID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'offline_checkin.conflict' AND metadata->>'batch_id' = $1`, []interface{}{pkg.BatchID}, 3)

	var batchStatus string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT status FROM offline_checkin_batches WHERE batch_id = $1`, pkg.BatchID).Scan(&batchStatus))
	assert.Equal(t, "conflict", batchStatus)

	replayed, err := service.SyncOfflineCheckins(ctx, staff, OfflineCheckinSyncRequest{
		BatchID:          pkg.BatchID,
		EventID:          event.EventID,
		DeviceID:         "gate-1",
		PackageSignature: pkg.PackageSignature,
		Scans:            scans,
	})
	require.NoError(t, err)
	assert.Equal(t, response.Accepted, replayed.Accepted)
	assert.Equal(t, response.Duplicate, replayed.Duplicate)
	assert.Equal(t, response.Conflict, replayed.Conflict)
	require.Len(t, replayed.Results, 4)
	assert.Equal(t, "offline_scan_event_mismatch", replayed.Results[1].ConflictReason)
	assert.Equal(t, otherTicket.TicketID, replayed.Results[1].TicketID)
	assert.NotEmpty(t, replayed.Results[1].Holder.DisplayName)
	assert.Equal(t, "ticket_token_claims_mismatch", replayed.Results[2].ConflictReason)
	assert.Equal(t, claimsMismatchTicket.TicketID, replayed.Results[2].TicketID)
	assert.NotEmpty(t, replayed.Results[2].Holder.DisplayName)
	assert.Equal(t, response.Results[3].CheckinID, replayed.Results[3].CheckinID)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1`, []interface{}{pkg.BatchID}, 4)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'offline_checkin.conflict' AND metadata->>'batch_id' = $1`, []interface{}{pkg.BatchID}, 3)
}

func TestSyncOfflineCheckinsRecordsMissingTicketForValidToken(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, _ := createOfflineSyncTicket(t, service, ctx, "Missing Offline Ticket", "E1001", "missing-offline-ticket")
	pkg, err := service.OfflineCheckinPackage(ctx, staff, event.EventID, "gate-1")
	require.NoError(t, err)
	missingToken, err := service.signer.Sign(TicketClaims{
		TicketID:   "tkt-offline-missing",
		EventID:    event.EventID,
		EmployeeID: "E1001",
	})
	require.NoError(t, err)

	response, err := service.SyncOfflineCheckins(ctx, staff, OfflineCheckinSyncRequest{
		BatchID:          pkg.BatchID,
		EventID:          event.EventID,
		DeviceID:         "gate-1",
		PackageSignature: pkg.PackageSignature,
		Scans:            []OfflineCheckinScanInput{{SignedToken: missingToken, ScannedAt: time.Now().UTC()}},
	})

	require.NoError(t, err)
	assert.Equal(t, 0, response.Accepted)
	assert.Equal(t, 0, response.Duplicate)
	assert.Equal(t, 1, response.Conflict)
	require.Len(t, response.Results, 1)
	assert.Equal(t, "conflict", response.Results[0].Status)
	assert.Equal(t, offlineConflictNotFound, response.Results[0].ConflictReason)
	assert.Equal(t, "tkt-offline-missing", response.Results[0].TicketID)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1 AND status = 'conflict' AND ticket_id IS NULL`, []interface{}{pkg.BatchID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'offline_checkin.conflict' AND entity_type = 'offline_checkin_batch' AND entity_id = $1`, []interface{}{pkg.BatchID}, 1)
}

func TestSyncOfflineCheckinsKeepsFirstCommitWinsForRepeatedScans(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, ticket := createOfflineSyncTicket(t, service, ctx, "First Commit Wins", "E1001", "first-commit-ticket")
	pkg, err := service.OfflineCheckinPackage(ctx, staff, event.EventID, "gate-1")
	require.NoError(t, err)
	firstScannedAt := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)

	req := OfflineCheckinSyncRequest{
		BatchID:          pkg.BatchID,
		EventID:          event.EventID,
		DeviceID:         "gate-1",
		PackageSignature: pkg.PackageSignature,
		Scans: []OfflineCheckinScanInput{
			{SignedToken: ticket.SignedToken, ScannedAt: firstScannedAt},
			{SignedToken: ticket.SignedToken, ScannedAt: firstScannedAt.Add(time.Minute)},
		},
	}
	response, err := service.SyncOfflineCheckins(ctx, staff, req)
	require.NoError(t, err)
	assert.Equal(t, 1, response.Accepted)
	assert.Equal(t, 1, response.Duplicate)
	assert.Equal(t, 0, response.Conflict)
	require.Len(t, response.Results, 2)
	assert.Equal(t, "accepted", response.Results[0].Status)
	assert.NotEmpty(t, response.Results[0].CheckinID)
	assert.True(t, response.Results[1].Duplicate)
	assert.Equal(t, staff.ID, response.Results[1].FirstScannedBy)
	assert.True(t, response.Results[1].FirstScannedAt.Equal(firstScannedAt))
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1 AND status = 'accepted'`, []interface{}{pkg.BatchID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1 AND status = 'duplicate'`, []interface{}{pkg.BatchID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'ticket.redeemed' AND entity_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'ticket.redeemed' AND aggregate_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertTicketOutboxPayload(t, service, ctx, "ticket.redeemed", ticket)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'offline_checkin.conflict' AND metadata->>'batch_id' = $1`, []interface{}{pkg.BatchID}, 1)

	replayed, err := service.SyncOfflineCheckins(ctx, staff, req)
	require.NoError(t, err)
	assert.Equal(t, response.Accepted, replayed.Accepted)
	assert.Equal(t, response.Duplicate, replayed.Duplicate)
	assert.Equal(t, response.Conflict, replayed.Conflict)
	require.Len(t, replayed.Results, 2)
	assert.Equal(t, response.Results[0].CheckinID, replayed.Results[0].CheckinID)
	assert.True(t, replayed.Results[1].Duplicate)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1`, []interface{}{pkg.BatchID}, 2)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'ticket.redeemed' AND entity_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'ticket.redeemed' AND aggregate_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'offline_checkin.conflict' AND metadata->>'batch_id' = $1`, []interface{}{pkg.BatchID}, 1)
}

func TestSyncOfflineCheckinsRejectsTamperedOrExpiredPackage(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, ticket := createOfflineSyncTicket(t, service, ctx, "Package Signature", "E1001", "package-signature-ticket")
	pkg, err := service.OfflineCheckinPackage(ctx, staff, event.EventID, "gate-1")
	require.NoError(t, err)
	require.NotEmpty(t, pkg.PackageSignature, "offline package signature must be returned")

	_, err = service.SyncOfflineCheckins(ctx, staff, OfflineCheckinSyncRequest{
		BatchID:          pkg.BatchID,
		EventID:          event.EventID,
		DeviceID:         "gate-1",
		PackageSignature: pkg.PackageSignature + "tampered",
		Scans:            []OfflineCheckinScanInput{{SignedToken: ticket.SignedToken, ScannedAt: now}},
	})
	require.Error(t, err, "expected tampered package error")
	assert.Equal(t, 400, ErrorStatus(err))
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1`, []interface{}{pkg.BatchID}, 0)

	service.now = func() time.Time { return pkg.ValidUntil.Add(time.Second) }
	_, err = service.SyncOfflineCheckins(ctx, staff, OfflineCheckinSyncRequest{
		BatchID:          pkg.BatchID,
		EventID:          event.EventID,
		DeviceID:         "gate-1",
		PackageSignature: pkg.PackageSignature,
		Scans:            []OfflineCheckinScanInput{{SignedToken: ticket.SignedToken, ScannedAt: now}},
	})
	require.Error(t, err, "expected expired package conflict")
	assert.Equal(t, 409, ErrorStatus(err))
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1`, []interface{}{pkg.BatchID}, 0)
}

func createOfflineSyncTicket(t *testing.T, service *Service, ctx context.Context, title string, employeeID string, idempotencyKey string) (EventSummary, Ticket) {
	t.Helper()
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    title,
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	booking, err := service.Book(ctx, Actor{ID: employeeID, Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: employeeID, IdempotencyKey: idempotencyKey})
	require.NoError(t, err)
	require.NotNil(t, booking.Ticket, "booking did not issue a ticket")
	return event, *booking.Ticket
}

func assertOfflineSyncCount(t *testing.T, service *Service, ctx context.Context, query string, args []interface{}, want int) {
	t.Helper()
	var got int
	require.NoError(t, service.db.QueryRow(ctx, query, args...).Scan(&got))
	assert.Equal(t, want, got, "row count for %q", query)
}
