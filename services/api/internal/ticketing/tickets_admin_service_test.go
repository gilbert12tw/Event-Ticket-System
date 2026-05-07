package ticketing

import (
	"context"
	"testing"
)

func TestGetTicketTokenVisibleOnlyToOwningEmployee(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()

	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Token Visibility Event",
		Capacity: 10,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 0, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := Actor{ID: "E1001", Role: RoleEmployee}
	booking, err := service.Book(ctx, owner, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "ticket-vis-1"})
	if err != nil {
		t.Fatal(err)
	}
	owned, err := service.GetTicket(ctx, owner, booking.Ticket.TicketID)
	if err != nil {
		t.Fatal(err)
	}
	if owned.SignedToken == "" || owned.QRPayload == "" {
		t.Fatalf("owning employee should receive token payload")
	}

	hr := Actor{ID: "hr-1", Role: RoleHRAdmin}
	hrView, err := service.GetTicket(ctx, hr, booking.Ticket.TicketID)
	if err != nil {
		t.Fatal(err)
	}
	if hrView.SignedToken != "" || hrView.QRPayload != "" {
		t.Fatalf("hr should receive sanitized ticket: %+v", hrView)
	}

	checkin := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	checkinView, err := service.GetTicket(ctx, checkin, booking.Ticket.TicketID)
	if err != nil {
		t.Fatal(err)
	}
	if checkinView.SignedToken != "" || checkinView.QRPayload != "" {
		t.Fatalf("checkin staff should receive sanitized ticket: %+v", checkinView)
	}

	activityAdmin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	adminView, err := service.GetTicket(ctx, activityAdmin, booking.Ticket.TicketID)
	if err != nil {
		t.Fatal(err)
	}
	if adminView.SignedToken != "" || adminView.QRPayload != "" {
		t.Fatalf("activity admin should receive sanitized ticket: %+v", adminView)
	}

	_, err = service.GetTicket(ctx, Actor{ID: "E1002", Role: RoleEmployee}, booking.Ticket.TicketID)
	if err == nil || ErrorStatus(err) != 403 {
		t.Fatalf("non-owner employee should be forbidden: %v", err)
	}
}

func TestRevokeTicketWritesOutbox(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()

	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Revocation Event",
		Capacity: 10,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 0, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "revoke-ticket-1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RevokeTicket(ctx, admin, booking.Ticket.TicketID, RevokeTicketRequest{Reason: "fraud detected"})
	if err != nil {
		t.Fatal(err)
	}

	var outboxCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'ticket.revoked' AND aggregate_id = $1`, booking.Ticket.TicketID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("ticket revoked outbox count = %d, want 1", outboxCount)
	}
}
