package ticketing

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceRejectsTamperedCheckinToken(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()

	_, err := service.CheckIn(context.Background(), Actor{ID: "staff-1", Role: RoleCheckinStaff}, CheckinRequest{SignedToken: "bad.token", EventID: "evt-invalid", DeviceID: "gate-1"})
	if err == nil || ErrorStatus(err) != 400 {
		t.Fatalf("expected bad token 400, got %v", err)
	}
}

func TestServiceRequiresEventIDForOnlineCheckin(t *testing.T) {
	service := &Service{}

	_, err := service.CheckIn(context.Background(), Actor{ID: "staff-1", Role: RoleCheckinStaff}, CheckinRequest{SignedToken: "any.token", DeviceID: "gate-1"})

	require.Error(t, err)
	assert.Equal(t, 400, ErrorStatus(err))
	assert.Equal(t, "event_id is required", ErrorMessage(err))
}

func TestServiceRejectsEventAndClaimsMismatchCheckinTokens(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Mismatch Check-in",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	otherEvent, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Other Mismatch Check-in",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "event-mismatch"})
	if err != nil {
		t.Fatal(err)
	}

	eventMismatch, err := service.CheckIn(ctx, staff, CheckinRequest{SignedToken: booking.Ticket.SignedToken, EventID: otherEvent.EventID, DeviceID: "gate-mismatch"})
	if err == nil || ErrorStatus(err) != 409 {
		t.Fatalf("expected event mismatch conflict, got %v", err)
	}
	if eventMismatch.Status != "rejected" || eventMismatch.ReasonCode != "event_mismatch" || eventMismatch.Holder.DisplayName != "Ariel Chen" {
		t.Fatalf("event mismatch response = %+v", eventMismatch)
	}

	mismatchToken, err := service.signer.Sign(TicketClaims{TicketID: booking.Ticket.TicketID, EventID: booking.Ticket.EventID, EmployeeID: "E1002"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.Exec(ctx, `UPDATE tickets SET signed_token_hash = $1 WHERE ticket_id = $2`, service.signer.HashToken(mismatchToken), booking.Ticket.TicketID); err != nil {
		t.Fatal(err)
	}
	claimsMismatch, err := service.CheckIn(ctx, staff, CheckinRequest{SignedToken: mismatchToken, EventID: event.EventID, DeviceID: "gate-mismatch"})
	if err == nil || ErrorStatus(err) != 400 {
		t.Fatalf("expected claims mismatch rejection, got %v", err)
	}
	if claimsMismatch.Status != "rejected" || claimsMismatch.ReasonCode != "ticket_token_claims_mismatch" || claimsMismatch.Holder.DisplayName != "Ariel Chen" {
		t.Fatalf("claims mismatch response = %+v", claimsMismatch)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM checkin_rejections WHERE ticket_id = $1`, booking.Ticket.TicketID, 2)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, booking.Ticket.TicketID, 0)
}

func TestServicePersistsHolderMismatchRejectionWithoutRedeeming(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Holder Mismatch",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "holder-mismatch"})
	if err != nil {
		t.Fatal(err)
	}

	rejected, err := service.CheckIn(ctx, staff, CheckinRequest{
		SignedToken:          booking.Ticket.SignedToken,
		EventID:              event.EventID,
		DeviceID:             "gate-holder",
		HolderMismatchReason: "photo ID does not match ticket holder",
	})
	if err == nil || ErrorStatus(err) != 409 {
		t.Fatalf("expected holder mismatch conflict, got %v", err)
	}
	if rejected.Status != "rejected" || rejected.ReasonCode != "holder_mismatch" || rejected.Holder.DisplayName != "Ariel Chen" {
		t.Fatalf("rejected response = %+v", rejected)
	}
	var rejectionCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM checkin_rejections WHERE ticket_id = $1 AND reason = 'holder_mismatch' AND staff_id = $2 AND device_id = $3`, booking.Ticket.TicketID, staff.ID, "gate-holder").Scan(&rejectionCount); err != nil {
		t.Fatal(err)
	}
	if rejectionCount != 1 {
		t.Fatalf("holder mismatch rejection count = %d, want 1", rejectionCount)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, booking.Ticket.TicketID, 0)
	logs, err := service.AuditLogs(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, AuditLogQuery{Action: "checkin.rejected", EntityID: booking.Ticket.TicketID})
	require.NoError(t, err)
	require.Len(t, logs, 1)
	metadata := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(logs[0].Metadata), &metadata))
	assert.Equal(t, event.EventID, metadata["event_id"])
	assert.Equal(t, "Holder Mismatch", metadata["event_title"])
	assert.Equal(t, "gate-holder", metadata["device_id"])
	assert.Equal(t, "holder_mismatch", metadata["reason"])
	assert.NotContains(t, logs[0].Metadata, "photo ID does not match ticket holder")
	assert.NotContains(t, logs[0].Metadata, "Ariel Chen")
	assert.NotContains(t, logs[0].Metadata, booking.Ticket.SignedToken)
	assert.NotContains(t, logs[0].Metadata, booking.Ticket.QRPayload)

	accepted, err := service.CheckIn(ctx, staff, CheckinRequest{SignedToken: booking.Ticket.SignedToken, EventID: event.EventID, DeviceID: "gate-holder"})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != "accepted" {
		t.Fatalf("accepted response = %+v", accepted)
	}
	assertTicketOutboxPayload(t, service, ctx, "ticket.redeemed", *booking.Ticket)
}

func TestCheckinTransferRejectionAuditOmitsFreeTextDetail(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Transfer Rejection",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	tx, err := service.db.Begin(ctx)
	require.NoError(t, err)
	defer rollback(ctx, tx)
	require.NoError(t, insertCheckinRejectionAuditTx(ctx, tx, Actor{ID: "staff-1", Role: RoleCheckinStaff}, "", event.EventID, "gate-transfer", "transfer_rejected", "Ariel Chen e1001@example.com raw.token.value"))
	require.NoError(t, tx.Commit(ctx))

	logs, err := service.AuditLogs(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, AuditLogQuery{Action: "checkin.rejected", EntityID: "gate-transfer"})
	require.NoError(t, err)
	require.Len(t, logs, 1)
	metadata := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(logs[0].Metadata), &metadata))
	assert.Equal(t, event.EventID, metadata["event_id"])
	assert.Equal(t, "Transfer Rejection", metadata["event_title"])
	assert.Equal(t, "gate-transfer", metadata["device_id"])
	assert.Equal(t, "transfer_rejected", metadata["reason"])
	assert.NotContains(t, logs[0].Metadata, "Ariel Chen")
	assert.NotContains(t, logs[0].Metadata, "e1001@example.com")
	assert.NotContains(t, logs[0].Metadata, "raw.token.value")
}

func TestServiceExpiresTicketDuringCheckin(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Expired Ticket Check-in",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "expired-ticket-booking"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.Exec(ctx, `UPDATE tickets SET expires_at = $1 WHERE ticket_id = $2`, now.Add(-time.Minute), booking.Ticket.TicketID); err != nil {
		t.Fatal(err)
	}

	rejected, err := service.CheckIn(ctx, Actor{ID: "staff-1", Role: RoleCheckinStaff}, CheckinRequest{SignedToken: booking.Ticket.SignedToken, EventID: event.EventID, DeviceID: "gate-expired"})
	if err == nil || ErrorStatus(err) != 409 {
		t.Fatalf("expected expired check-in 409, got %v", err)
	}
	if rejected.Status != "rejected" || rejected.ReasonCode != "expired_ticket" || rejected.Holder.DisplayName != "Ariel Chen" {
		t.Fatalf("expired response = %+v", rejected)
	}

	var status string
	if err := service.db.QueryRow(ctx, `SELECT status FROM tickets WHERE ticket_id = $1`, booking.Ticket.TicketID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != TicketExpired {
		t.Fatalf("ticket status = %q, want %q", status, TicketExpired)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'ticket.expired' AND entity_id = $1`, booking.Ticket.TicketID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'ticket.expired' AND aggregate_id = $1`, booking.Ticket.TicketID, 1)
	assertTicketOutboxPayload(t, service, ctx, "ticket.expired", *booking.Ticket)
}

func TestServiceRejectsRevokedTicketDuringCheckin(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Revoked Ticket Check-in",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "revoked-ticket-booking"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RevokeTicket(ctx, admin, booking.Ticket.TicketID, RevokeTicketRequest{Reason: "manual fraud review"}); err != nil {
		t.Fatal(err)
	}

	rejected, err := service.CheckIn(ctx, Actor{ID: "staff-1", Role: RoleCheckinStaff}, CheckinRequest{SignedToken: booking.Ticket.SignedToken, EventID: event.EventID, DeviceID: "gate-revoked"})
	if err == nil || ErrorStatus(err) != 409 {
		t.Fatalf("expected revoked check-in 409, got %v", err)
	}
	if rejected.Status != "rejected" || rejected.ReasonCode != "revoked_ticket" || rejected.Holder.DisplayName != "Ariel Chen" {
		t.Fatalf("revoked response = %+v", rejected)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM checkin_rejections WHERE ticket_id = $1 AND reason = 'revoked_ticket'`, booking.Ticket.TicketID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM checkin_records WHERE ticket_id = $1`, booking.Ticket.TicketID, 0)
}
