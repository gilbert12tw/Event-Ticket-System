package ticketing

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"event-ticket-system/internal/postgres"
	"event-ticket-system/internal/traceid"
)

func TestServiceBookingAndCheckinFlow(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	service, cleanup := newIntegrationServiceWithLogger(t, logger)
	defer cleanup()
	ctx := traceid.WithContext(context.Background(), "trace-integration-1")

	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	hr := Actor{ID: "hr-1", Role: RoleHRAdmin}
	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:       "Engineering Demo Day",
		Description: "Demo",
		Location:    "Taipei HQ",
		Capacity:    1,
		Status:      EventStatusPublished,
		Rule:        RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'event.created' AND entity_id = $1`, event.EventID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM eligibility_rule_versions WHERE event_id = $1 AND version = 1`, event.EventID, 1)

	eligible, err := service.CheckEligibility(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, "E1001")
	if err != nil {
		t.Fatal(err)
	}
	if eligible["eligible"] != true {
		t.Fatalf("expected eligible, got %+v", eligible)
	}

	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "idem-1"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Registration.Status != RegistrationConfirmed || first.Ticket == nil {
		t.Fatalf("first booking = %+v", first)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'booking.confirmed' AND aggregate_id = $1`, first.Registration.RegistrationID, 1)
	var rawTicketCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE signed_token <> '' OR qr_payload <> ''`).Scan(&rawTicketCount); err != nil {
		t.Fatal(err)
	}
	if rawTicketCount != 0 {
		t.Fatalf("expected no raw ticket tokens stored, got %d", rawTicketCount)
	}
	eventList, err := service.ListEvents(ctx, Actor{ID: "E1001", Role: RoleEmployee}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(eventList) != 1 || eventList[0].CurrentUserTicket == nil {
		t.Fatalf("expected event summary with current user ticket status, got %+v", eventList)
	}
	if eventList[0].CurrentUserTicket.SignedToken != "" || eventList[0].CurrentUserTicket.QRPayload != "" {
		t.Fatalf("event summary leaked ticket token: %+v", eventList[0].CurrentUserTicket)
	}
	if _, err := service.ListEvents(ctx, Actor{ID: "E2001", Role: RoleEmployee}, "E1001"); err == nil || ErrorStatus(err) != 403 {
		t.Fatalf("expected cross-employee event list 403, got %v", err)
	}
	retry, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "idem-1"})
	if err != nil {
		t.Fatal(err)
	}
	if retry.Registration.RegistrationID != first.Registration.RegistrationID || retry.Ticket == nil || retry.Ticket.TicketID != first.Ticket.TicketID {
		t.Fatalf("idempotent retry = %+v, want registration %s ticket %s", retry, first.Registration.RegistrationID, first.Ticket.TicketID)
	}
	employeeRetry, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "idem-1-different"})
	if err != nil {
		t.Fatal(err)
	}
	if employeeRetry.Registration.RegistrationID != first.Registration.RegistrationID || employeeRetry.Ticket == nil || employeeRetry.Ticket.TicketID != first.Ticket.TicketID {
		t.Fatalf("same employee retry = %+v, want registration %s ticket %s", employeeRetry, first.Registration.RegistrationID, first.Ticket.TicketID)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND employee_id = 'E1001'`, event.EventID, 1)

	second, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "idem-2"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Registration.Status != RegistrationWaitlisted || second.Ticket != nil {
		t.Fatalf("second booking = %+v", second)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM tickets WHERE registration_id = $1`, second.Registration.RegistrationID, 0)

	if _, err := service.Book(ctx, Actor{ID: "E2001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E2001", IdempotencyKey: "idem-3"}); err == nil || ErrorStatus(err) != 403 {
		t.Fatalf("expected ineligible 403, got %v", err)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND employee_id = 'E2001'`, event.EventID, 0)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM tickets WHERE event_id = $1 AND employee_id = 'E2001'`, event.EventID, 0)

	checkin, err := service.CheckIn(ctx, staff, CheckinRequest{SignedToken: first.Ticket.SignedToken, DeviceID: "gate-1"})
	if err != nil {
		t.Fatal(err)
	}
	if checkin.Status != "accepted" || checkin.TicketID != first.Ticket.TicketID {
		t.Fatalf("checkin = %+v", checkin)
	}
	duplicate, err := service.CheckIn(ctx, staff, CheckinRequest{SignedToken: first.Ticket.SignedToken, DeviceID: "gate-1"})
	if err == nil || ErrorStatus(err) != 409 {
		t.Fatalf("expected duplicate checkin 409, got %v", err)
	}
	if !duplicate.Duplicate || duplicate.FirstScannedBy != staff.ID || duplicate.FirstScannedAt.IsZero() {
		t.Fatalf("duplicate details = %+v", duplicate)
	}

	reports, err := service.Reports(ctx, hr)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].ConfirmedCount != 1 || reports[0].WaitlistCount != 1 || reports[0].CheckinCount != 1 {
		t.Fatalf("reports = %+v", reports)
	}
	audits, err := service.AuditLogs(ctx, hr)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) < 3 {
		t.Fatalf("expected audit logs, got %+v", audits)
	}
	foundConflictAudit := false
	for _, audit := range audits {
		if audit.Action == "checkin.conflict" {
			foundConflictAudit = true
		}
	}
	if !foundConflictAudit {
		t.Fatalf("expected duplicate check-in conflict audit, got %+v", audits)
	}
	assertLogContains(t, logs.String(),
		`"trace_id":"trace-integration-1"`,
		`"action":"event.created"`,
		`"action":"booking.confirmed"`,
		`"action":"ticket.redeemed"`,
		`"status":"success"`,
	)
}

func TestServiceRejectsTamperedCheckinToken(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()

	_, err := service.CheckIn(context.Background(), Actor{ID: "staff-1", Role: RoleCheckinStaff}, CheckinRequest{SignedToken: "bad.token", DeviceID: "gate-1"})
	if err == nil || ErrorStatus(err) != 400 {
		t.Fatalf("expected bad token 400, got %v", err)
	}
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
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
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

	_, err = service.CheckIn(ctx, Actor{ID: "staff-1", Role: RoleCheckinStaff}, CheckinRequest{SignedToken: booking.Ticket.SignedToken, DeviceID: "gate-expired"})
	if err == nil || ErrorStatus(err) != 409 {
		t.Fatalf("expected expired check-in 409, got %v", err)
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
}

func TestServiceEligibilityUpdateCreatesImpactReviews(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Eligibility Impact",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM eligibility_rule_versions WHERE event_id = $1 AND version = 1`, event.EventID, 1)

	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "impact-booking"})
	if err != nil {
		t.Fatal(err)
	}
	if booking.Ticket == nil {
		t.Fatal("expected confirmed booking to issue a ticket")
	}

	if _, err := service.UpdateEligibility(ctx, admin, event.EventID, UpdateEligibilityRequest{
		Rule: RuleInput{Department: "Legal", Site: "Nowhere", MinGrade: 99, EmploymentStatus: "active"},
	}); err == nil || ErrorStatus(err) != 409 {
		t.Fatalf("expected zero-match update conflict, got %v", err)
	}

	updated, err := service.UpdateEligibility(ctx, admin, event.EventID, UpdateEligibilityRequest{
		Rule: RuleInput{Department: "Sales", Site: "Taipei", MinGrade: 4, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.MatchCount != 1 || updated.ZeroMatch {
		t.Fatalf("eligibility update = %+v, want one non-zero match", updated)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM eligibility_rule_versions WHERE event_id = $1`, event.EventID, 2)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM eligibility_impact_reviews WHERE event_id = $1 AND employee_id = 'E1001' AND status = 'pending'`, event.EventID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'eligibility.impact_review.created' AND aggregate_id = $1`, event.EventID, 1)

	if _, err := service.UpdateEligibility(ctx, admin, event.EventID, UpdateEligibilityRequest{
		Rule:           RuleInput{Department: "Legal", Site: "Nowhere", MinGrade: 99, EmploymentStatus: "active"},
		AllowZeroMatch: true,
	}); err != nil {
		t.Fatal(err)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM eligibility_impact_reviews WHERE event_id = $1 AND employee_id = 'E1001' AND status = 'pending'`, event.EventID, 1)
}

func TestServiceRejectsIdempotencyKeyCollisionAcrossEmployees(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Collision Test",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "shared-key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "shared-key"}); err == nil || ErrorStatus(err) != 409 {
		t.Fatalf("expected idempotency collision 409, got %v", err)
	}
}

func TestServiceConcurrentBookingsDoNotOversellLastSeat(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Last Seat Race",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan BookingResponse, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, employeeID := range []string{"E1001", "E1002"} {
		employeeID := employeeID
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := service.Book(ctx, Actor{ID: employeeID, Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: employeeID, IdempotencyKey: "race-" + employeeID})
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("booking failed: %v", err)
	}
	var confirmed, waitlisted int
	for result := range results {
		switch result.Registration.Status {
		case RegistrationConfirmed:
			confirmed++
		case RegistrationWaitlisted:
			waitlisted++
		}
	}
	if confirmed != 1 || waitlisted != 1 {
		t.Fatalf("confirmed=%d waitlisted=%d", confirmed, waitlisted)
	}
}

func newIntegrationService(t *testing.T) (*Service, func()) {
	return newIntegrationServiceWithLogger(t, slog.Default())
}

func newIntegrationServiceWithLogger(t *testing.T, logger *slog.Logger) (*Service, func()) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	suffix := time.Now().UnixNano()
	if _, err := pool.Exec(ctx, fmt.Sprintf("TRUNCATE outbox_events, audit_logs, checkin_records, tickets, registrations, eligibility_rules, events, employees RESTART IDENTITY CASCADE")); err != nil {
		t.Fatal(err)
	}

	service := NewService(pool, NewSigner(fmt.Sprintf("secret-%d", suffix)), logger)
	return service, pool.Close
}

func assertRowCount(t *testing.T, service *Service, ctx context.Context, query string, arg interface{}, want int) {
	t.Helper()
	var got int
	if err := service.db.QueryRow(ctx, query, arg).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("row count for %q = %d, want %d", query, got, want)
	}
}

func assertLogContains(t *testing.T, logs string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(logs, fragment) {
			t.Fatalf("logs %q do not contain %q", logs, fragment)
		}
	}
}
