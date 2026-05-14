package ticketing

import (
	"context"
	"testing"
	"time"
)

func TestSyncOfflineCheckinsValidatesBatchOwnershipBeforeScans(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

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
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.SyncOfflineCheckins(ctx, tt.actor, OfflineCheckinSyncRequest{
				BatchID:          pkg.BatchID,
				EventID:          tt.eventID,
				DeviceID:         tt.deviceID,
				PackageSignature: pkg.PackageSignature,
				Scans:            []OfflineCheckinScanInput{{SignedToken: ticket.SignedToken, ScannedAt: time.Now().UTC()}},
			})
			if err == nil || ErrorStatus(err) != tt.wantError {
				t.Fatalf("expected %d ownership error, got %v", tt.wantError, err)
			}
			assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{ticket.TicketID}, 0)
			assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1`, []interface{}{pkg.BatchID}, 0)
			assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'offline_checkin.conflict' AND metadata->>'batch_id' = $1`, []interface{}{pkg.BatchID}, 0)
		})
	}
}

func TestSyncOfflineCheckinsPreservesPerScanConflictsAndAudits(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, ticket := createOfflineSyncTicket(t, service, ctx, "Per Scan Conflicts", "E1001", "conflict-ticket")
	_, otherTicket := createOfflineSyncTicket(t, service, ctx, "Mismatched Event Token", "E1002", "mismatch-ticket")
	_, claimsMismatchTicket := createOfflineSyncTicket(t, service, ctx, "Mismatched Claims Token", "E1001", "claims-mismatch-ticket")
	claimsMismatchToken, err := service.signer.Sign(TicketClaims{TicketID: claimsMismatchTicket.TicketID, EventID: claimsMismatchTicket.EventID, EmployeeID: "E1002"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.Exec(ctx, `UPDATE tickets SET signed_token_hash = $1 WHERE ticket_id = $2`, service.signer.HashToken(claimsMismatchToken), claimsMismatchTicket.TicketID); err != nil {
		t.Fatal(err)
	}
	pkg, err := service.OfflineCheckinPackage(ctx, staff, event.EventID, "gate-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Tickets) != 1 || pkg.Tickets[0].Holder.DisplayName == "" || pkg.Tickets[0].Holder.Department == "" || pkg.Tickets[0].Holder.City == "" {
		t.Fatalf("offline package holder data = %+v", pkg.Tickets)
	}

	response, err := service.SyncOfflineCheckins(ctx, staff, OfflineCheckinSyncRequest{
		BatchID:          pkg.BatchID,
		EventID:          event.EventID,
		DeviceID:         "gate-1",
		PackageSignature: pkg.PackageSignature,
		Scans: []OfflineCheckinScanInput{
			{SignedToken: "bad.token", ScannedAt: time.Now().UTC().Add(time.Second)},
			{SignedToken: otherTicket.SignedToken, ScannedAt: time.Now().UTC().Add(2 * time.Second)},
			{SignedToken: claimsMismatchToken, ScannedAt: time.Now().UTC().Add(3 * time.Second)},
			{SignedToken: ticket.SignedToken, ScannedAt: time.Now().UTC().Add(4 * time.Second)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Accepted != 1 || response.Duplicate != 0 || response.Conflict != 3 || len(response.Results) != 4 {
		t.Fatalf("response counts = %+v", response)
	}
	if response.Results[0].Status != "conflict" || response.Results[0].ConflictReason != "invalid_ticket_token" {
		t.Fatalf("invalid token result = %+v", response.Results[0])
	}
	if response.Results[1].Status != "conflict" || response.Results[1].ConflictReason != "offline_scan_event_mismatch" {
		t.Fatalf("mismatched token result = %+v", response.Results[1])
	}
	if response.Results[2].Status != "conflict" || response.Results[2].ConflictReason != "ticket_token_claims_mismatch" {
		t.Fatalf("claims mismatch result = %+v", response.Results[2])
	}
	if response.Results[3].Status != "accepted" || response.Results[3].TicketID != ticket.TicketID {
		t.Fatalf("accepted token result = %+v", response.Results[3])
	}

	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{otherTicket.TicketID}, 0)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{claimsMismatchTicket.TicketID}, 0)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1 AND status = 'accepted'`, []interface{}{pkg.BatchID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1 AND status = 'conflict'`, []interface{}{pkg.BatchID}, 3)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1 AND status = 'conflict' AND ticket_id IS NULL`, []interface{}{pkg.BatchID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'offline_checkin.conflict' AND metadata->>'batch_id' = $1`, []interface{}{pkg.BatchID}, 3)

	var batchStatus string
	if err := service.db.QueryRow(ctx, `SELECT status FROM offline_checkin_batches WHERE batch_id = $1`, pkg.BatchID).Scan(&batchStatus); err != nil {
		t.Fatal(err)
	}
	if batchStatus != "conflict" {
		t.Fatalf("batch status = %s, want conflict", batchStatus)
	}
}

func TestSyncOfflineCheckinsKeepsFirstCommitWinsForRepeatedScans(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, ticket := createOfflineSyncTicket(t, service, ctx, "First Commit Wins", "E1001", "first-commit-ticket")
	pkg, err := service.OfflineCheckinPackage(ctx, staff, event.EventID, "gate-1")
	if err != nil {
		t.Fatal(err)
	}
	firstScannedAt := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)

	response, err := service.SyncOfflineCheckins(ctx, staff, OfflineCheckinSyncRequest{
		BatchID:          pkg.BatchID,
		EventID:          event.EventID,
		DeviceID:         "gate-1",
		PackageSignature: pkg.PackageSignature,
		Scans: []OfflineCheckinScanInput{
			{SignedToken: ticket.SignedToken, ScannedAt: firstScannedAt},
			{SignedToken: ticket.SignedToken, ScannedAt: firstScannedAt.Add(time.Minute)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Accepted != 1 || response.Duplicate != 1 || response.Conflict != 0 || len(response.Results) != 2 {
		t.Fatalf("response counts = %+v", response)
	}
	if response.Results[0].Status != "accepted" || response.Results[0].CheckinID == "" {
		t.Fatalf("first scan result = %+v", response.Results[0])
	}
	if !response.Results[1].Duplicate || response.Results[1].FirstScannedBy != staff.ID || !response.Results[1].FirstScannedAt.Equal(firstScannedAt) {
		t.Fatalf("duplicate scan result = %+v", response.Results[1])
	}
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1 AND status = 'accepted'`, []interface{}{pkg.BatchID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1 AND status = 'duplicate'`, []interface{}{pkg.BatchID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'ticket.redeemed' AND entity_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'ticket.redeemed' AND aggregate_id = $1`, []interface{}{ticket.TicketID}, 1)
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'offline_checkin.conflict' AND metadata->>'batch_id' = $1`, []interface{}{pkg.BatchID}, 1)
}

func TestSyncOfflineCheckinsRejectsTamperedOrExpiredPackage(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, ticket := createOfflineSyncTicket(t, service, ctx, "Package Signature", "E1001", "package-signature-ticket")
	pkg, err := service.OfflineCheckinPackage(ctx, staff, event.EventID, "gate-1")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.PackageSignature == "" {
		t.Fatal("offline package signature must be returned")
	}

	_, err = service.SyncOfflineCheckins(ctx, staff, OfflineCheckinSyncRequest{
		BatchID:          pkg.BatchID,
		EventID:          event.EventID,
		DeviceID:         "gate-1",
		PackageSignature: pkg.PackageSignature + "tampered",
		Scans:            []OfflineCheckinScanInput{{SignedToken: ticket.SignedToken, ScannedAt: now}},
	})
	if err == nil || ErrorStatus(err) != 400 {
		t.Fatalf("expected tampered package error, got %v", err)
	}
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1`, []interface{}{pkg.BatchID}, 0)

	service.now = func() time.Time { return pkg.ValidUntil.Add(time.Second) }
	_, err = service.SyncOfflineCheckins(ctx, staff, OfflineCheckinSyncRequest{
		BatchID:          pkg.BatchID,
		EventID:          event.EventID,
		DeviceID:         "gate-1",
		PackageSignature: pkg.PackageSignature,
		Scans:            []OfflineCheckinScanInput{{SignedToken: ticket.SignedToken, ScannedAt: now}},
	})
	if err == nil || ErrorStatus(err) != 409 {
		t.Fatalf("expected expired package conflict, got %v", err)
	}
	assertOfflineSyncCount(t, service, ctx, `SELECT count(*) FROM offline_checkin_scans WHERE batch_id = $1`, []interface{}{pkg.BatchID}, 0)
}

func createOfflineSyncTicket(t *testing.T, service *Service, ctx context.Context, title string, employeeID string, idempotencyKey string) (EventSummary, Ticket) {
	t.Helper()
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    title,
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	booking, err := service.Book(ctx, Actor{ID: employeeID, Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: employeeID, IdempotencyKey: idempotencyKey})
	if err != nil {
		t.Fatal(err)
	}
	if booking.Ticket == nil {
		t.Fatalf("booking did not issue a ticket: %+v", booking)
	}
	return event, *booking.Ticket
}

func assertOfflineSyncCount(t *testing.T, service *Service, ctx context.Context, query string, args []interface{}, want int) {
	t.Helper()
	var got int
	if err := service.db.QueryRow(ctx, query, args...).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("row count for %q = %d, want %d", query, got, want)
	}
}
