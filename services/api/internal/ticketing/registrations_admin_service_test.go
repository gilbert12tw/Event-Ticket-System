package ticketing

import (
	"context"
	"testing"
)

func TestCancelRegistrationPromotesWaitlistInSingleTransaction(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Promotion Transactional Test",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}

	confirmed, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "book-1"})
	if err != nil {
		t.Fatal(err)
	}
	waitlist, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "book-2"})
	if err != nil {
		t.Fatal(err)
	}

	cancelled, err := service.CancelRegistration(ctx, admin, event.EventID, confirmed.Registration.RegistrationID, CancelRegistrationRequest{
		IdempotencyKey: "cancel-1",
		Reason:         "speaker change",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Registration.Status != RegistrationCancelled {
		t.Fatalf("cancelled registration status = %s, want %s", cancelled.Registration.Status, RegistrationCancelled)
	}
	if cancelled.RemainingCapacity != 0 {
		t.Fatalf("remaining capacity = %d, want %d", cancelled.RemainingCapacity, 0)
	}
	if cancelled.Ticket != nil && (cancelled.Ticket.SignedToken != "" || cancelled.Ticket.QRPayload != "") {
		t.Fatalf("admin cancellation response should not expose reusable token: %+v", cancelled.Ticket)
	}

	var promotedStatus string
	if err := service.db.QueryRow(ctx, `SELECT status FROM registrations WHERE registration_id = $1`, waitlist.Registration.RegistrationID).Scan(&promotedStatus); err != nil {
		t.Fatal(err)
	}
	if promotedStatus != RegistrationConfirmed {
		t.Fatalf("promoted registration status = %s, want %s", promotedStatus, RegistrationConfirmed)
	}
	var promotedTicketCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE registration_id = $1`, waitlist.Registration.RegistrationID).Scan(&promotedTicketCount); err != nil {
		t.Fatal(err)
	}
	if promotedTicketCount != 1 {
		t.Fatalf("promoted ticket count = %d, want %d", promotedTicketCount, 1)
	}

	var canceledOutboxCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'registration.cancelled' AND aggregate_id = $1`, confirmed.Registration.RegistrationID).Scan(&canceledOutboxCount); err != nil {
		t.Fatal(err)
	}
	if canceledOutboxCount != 1 {
		t.Fatalf("registration cancelled outbox count = %d, want %d", canceledOutboxCount, 1)
	}
	var promotedOutboxCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'waitlist.promoted' AND aggregate_id = $1`, waitlist.Registration.RegistrationID).Scan(&promotedOutboxCount); err != nil {
		t.Fatal(err)
	}
	if promotedOutboxCount != 1 {
		t.Fatalf("waitlist promoted outbox count = %d, want %d", promotedOutboxCount, 1)
	}
}

func TestCancelRegistrationRetryIsSafeByCancelIdempotencyKey(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Idempotent Cancel Test",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}

	confirmed, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "idem-book-1"})
	if err != nil {
		t.Fatal(err)
	}
	waitlist, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "idem-book-2"})
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.CancelRegistration(ctx, admin, event.EventID, confirmed.Registration.RegistrationID, CancelRegistrationRequest{
		IdempotencyKey: "cancel-idem-1",
		Reason:         "full cancellation",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Registration.Status != RegistrationCancelled {
		t.Fatalf("first cancellation status = %s, want %s", first.Registration.Status, RegistrationCancelled)
	}

	second, err := service.CancelRegistration(ctx, admin, event.EventID, confirmed.Registration.RegistrationID, CancelRegistrationRequest{
		IdempotencyKey: "cancel-idem-1",
		Reason:         "retry",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Message != "registration already cancelled" {
		t.Fatalf("second cancellation message = %s, want %s", second.Message, "registration already cancelled")
	}

	var cancellationAuditCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'registration.cancelled' AND entity_id = $1`, confirmed.Registration.RegistrationID).Scan(&cancellationAuditCount); err != nil {
		t.Fatal(err)
	}
	if cancellationAuditCount != 1 {
		t.Fatalf("registration cancelled audit count = %d, want %d", cancellationAuditCount, 1)
	}

	var promotionAuditCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'waitlist.promoted' AND entity_id = $1`, waitlist.Registration.RegistrationID).Scan(&promotionAuditCount); err != nil {
		t.Fatal(err)
	}
	if promotionAuditCount != 1 {
		t.Fatalf("waitlist promoted audit count = %d, want %d", promotionAuditCount, 1)
	}
	var promotionOutboxCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'waitlist.promoted' AND aggregate_id = $1`, waitlist.Registration.RegistrationID).Scan(&promotionOutboxCount); err != nil {
		t.Fatal(err)
	}
	if promotionOutboxCount != 1 {
		t.Fatalf("waitlist promoted outbox count = %d, want %d", promotionOutboxCount, 1)
	}
	var cancelledOutboxCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'registration.cancelled' AND aggregate_id = $1`, confirmed.Registration.RegistrationID).Scan(&cancelledOutboxCount); err != nil {
		t.Fatal(err)
	}
	if cancelledOutboxCount != 1 {
		t.Fatalf("registration cancelled outbox count = %d, want %d", cancelledOutboxCount, 1)
	}
}

func TestPromoteWaitlistDoesNotExposeTicketTokenToAdmin(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Sanitized Promotion",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "sanitize-book-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "sanitize-book-2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.Exec(ctx, `UPDATE events SET capacity = 2 WHERE event_id = $1`, event.EventID); err != nil {
		t.Fatal(err)
	}

	promoted, err := service.PromoteWaitlist(ctx, admin, event.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Promoted == nil || promoted.Promoted.Ticket == nil {
		t.Fatalf("expected promoted registration with ticket: %+v", promoted)
	}
	if promoted.Promoted.Ticket.SignedToken != "" || promoted.Promoted.Ticket.QRPayload != "" {
		t.Fatalf("admin promotion response should not expose reusable token: %+v", promoted.Promoted.Ticket)
	}
}
