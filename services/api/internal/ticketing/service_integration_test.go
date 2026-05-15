package ticketing

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"event-ticket-system/internal/postgres"
	"event-ticket-system/internal/traceid"
)

func TestServiceBookingAndCheckinFlow(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	service, cleanup := newIntegrationServiceWithLogger(t, logger)
	defer cleanup()
	ctx := traceid.WithContext(context.Background(), "trace-integration-1")

	require.NoError(t, service.SeedDemoData(ctx))
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
	require.NoError(t, err)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'event.created' AND entity_id = $1`, event.EventID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM eligibility_rule_versions WHERE event_id = $1 AND version = 1`, event.EventID, 1)

	eligible, err := service.CheckEligibility(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, "")
	require.NoError(t, err)
	assert.Equal(t, true, eligible["eligible"])

	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "idem-1"})
	require.NoError(t, err)
	require.Equal(t, RegistrationConfirmed, first.Registration.Status)
	require.NotNil(t, first.Ticket)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'booking.confirmed' AND aggregate_id = $1`, first.Registration.RegistrationID, 1)
	var rawTicketCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE signed_token <> '' OR qr_payload <> ''`).Scan(&rawTicketCount))
	assert.Equal(t, 0, rawTicketCount, "expected no raw ticket tokens stored")
	eventList, err := service.ListEvents(ctx, Actor{ID: "E1001", Role: RoleEmployee}, "")
	require.NoError(t, err)
	require.Len(t, eventList, 1)
	require.NotNil(t, eventList[0].CurrentUserTicket)
	assert.Empty(t, eventList[0].CurrentUserTicket.SignedToken)
	assert.Empty(t, eventList[0].CurrentUserTicket.QRPayload)
	_, err = service.ListEvents(ctx, Actor{ID: "E2001", Role: RoleEmployee}, "E1001")
	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))
	retry, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "idem-1"})
	require.NoError(t, err)
	assert.Equal(t, first.Registration.RegistrationID, retry.Registration.RegistrationID)
	require.NotNil(t, retry.Ticket)
	assert.Equal(t, first.Ticket.TicketID, retry.Ticket.TicketID)
	employeeRetry, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "idem-1-different"})
	require.NoError(t, err)
	assert.Equal(t, first.Registration.RegistrationID, employeeRetry.Registration.RegistrationID)
	require.NotNil(t, employeeRetry.Ticket)
	assert.Equal(t, first.Ticket.TicketID, employeeRetry.Ticket.TicketID)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND employee_id = 'E1001'`, event.EventID, 1)

	second, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "idem-2"})
	require.NoError(t, err)
	assert.Equal(t, RegistrationWaitlisted, second.Registration.Status)
	assert.Nil(t, second.Ticket)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM tickets WHERE registration_id = $1`, second.Registration.RegistrationID, 0)

	_, err = service.Book(ctx, Actor{ID: "E2001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E2001", IdempotencyKey: "idem-3"})
	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND employee_id = 'E2001'`, event.EventID, 0)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM tickets WHERE event_id = $1 AND employee_id = 'E2001'`, event.EventID, 0)

	checkin, err := service.CheckIn(ctx, staff, CheckinRequest{SignedToken: first.Ticket.SignedToken, DeviceID: "gate-1"})
	require.NoError(t, err)
	assert.Equal(t, "accepted", checkin.Status)
	assert.Equal(t, first.Ticket.TicketID, checkin.TicketID)
	assert.Equal(t, "Ariel Chen", checkin.Holder.DisplayName)
	assert.Equal(t, "Engineering", checkin.Holder.Department)
	assert.Equal(t, "Taipei", checkin.Holder.City)
	duplicate, err := service.CheckIn(ctx, staff, CheckinRequest{SignedToken: first.Ticket.SignedToken, DeviceID: "gate-1"})
	require.Error(t, err)
	assert.Equal(t, 409, ErrorStatus(err))
	assert.True(t, duplicate.Duplicate)
	assert.Equal(t, staff.ID, duplicate.FirstScannedBy)
	assert.False(t, duplicate.FirstScannedAt.IsZero())

	reports, err := service.Reports(ctx, hr)
	require.NoError(t, err)
	require.Len(t, reports, 1)
	assert.Equal(t, 1, reports[0].ConfirmedCount)
	assert.Equal(t, 1, reports[0].WaitlistCount)
	assert.Equal(t, 1, reports[0].CheckinCount)
	audits, err := service.AuditLogs(ctx, hr)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(audits), 3)
	foundConflictAudit := false
	for _, audit := range audits {
		if audit.Action == "checkin.conflict" {
			foundConflictAudit = true
		}
	}
	assert.True(t, foundConflictAudit, "expected duplicate check-in conflict audit")
	assertLogContains(t, logs.String(),
		`"trace_id":"trace-integration-1"`,
		`"action":"event.created"`,
		`"action":"booking.confirmed"`,
		`"action":"ticket.redeemed"`,
		`"status":"success"`,
	)
}

func TestServiceEligibilityUpdateCreatesImpactReviews(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Eligibility Impact",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM eligibility_rule_versions WHERE event_id = $1 AND version = 1`, event.EventID, 1)

	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "impact-booking"})
	require.NoError(t, err)
	require.NotNil(t, booking.Ticket, "expected confirmed booking to issue a ticket")

	_, err = service.UpdateEligibility(ctx, admin, event.EventID, UpdateEligibilityRequest{
		Rule: RuleInput{Department: "Legal", Site: "Nowhere", MinGrade: 99, EmploymentStatus: "active"},
	})
	require.Error(t, err)
	assert.Equal(t, 409, ErrorStatus(err))

	updated, err := service.UpdateEligibility(ctx, admin, event.EventID, UpdateEligibilityRequest{
		Rule: RuleInput{Department: "Sales", Site: "Taipei", MinGrade: 4, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, updated.MatchCount)
	assert.False(t, updated.ZeroMatch)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM eligibility_rule_versions WHERE event_id = $1`, event.EventID, 2)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM eligibility_impact_reviews WHERE event_id = $1 AND employee_id = 'E1001' AND status = 'pending'`, event.EventID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'eligibility.impact_review.created' AND aggregate_id = $1`, event.EventID, 1)

	_, err = service.UpdateEligibility(ctx, admin, event.EventID, UpdateEligibilityRequest{
		Rule:           RuleInput{Department: "Legal", Site: "Nowhere", MinGrade: 99, EmploymentStatus: "active"},
		AllowZeroMatch: true,
	})
	require.NoError(t, err)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM eligibility_impact_reviews WHERE event_id = $1 AND employee_id = 'E1001' AND status = 'pending'`, event.EventID, 1)
}

func TestServiceRejectsIdempotencyKeyCollisionAcrossEmployees(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Collision Test",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "shared-key"})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "shared-key"})
	require.Error(t, err)
	assert.Equal(t, 409, ErrorStatus(err))
}

func TestServiceConcurrentBookingsDoNotOversellLastSeat(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Last Seat Race",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

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
		require.NoError(t, err, "booking failed")
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
	assert.Equal(t, 1, confirmed)
	assert.Equal(t, 1, waitlisted)
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
	pool, cleanup := newTicketingTestPool(t, ctx, databaseURL)
	if err := postgres.Migrate(ctx, pool); err != nil {
		cleanup()
		require.NoError(t, err)
	}
	suffix := time.Now().UnixNano()
	if _, err := pool.Exec(ctx, "TRUNCATE outbox_events, audit_logs, checkin_records, tickets, registrations, eligibility_rules, events, employees RESTART IDENTITY CASCADE"); err != nil {
		cleanup()
		require.NoError(t, err)
	}

	service := NewService(pool, NewSigner(fmt.Sprintf("secret-%d", suffix)), logger)
	return service, cleanup
}

func newTicketingTestPool(t *testing.T, ctx context.Context, databaseURL string) (*pgxpool.Pool, func()) {
	t.Helper()

	adminPool, err := postgres.Connect(ctx, databaseURL)
	require.NoError(t, err)

	schema := fmt.Sprintf("ticketing_test_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", quotedSchema)); err != nil {
		adminPool.Close()
		require.NoError(t, err)
	}

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		adminPool.Close()
		require.NoError(t, err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		adminPool.Close()
		require.NoError(t, err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		adminPool.Close()
		require.NoError(t, err)
	}

	return pool, func() {
		pool.Close()
		dropCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = adminPool.Exec(dropCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", quotedSchema))
		adminPool.Close()
	}
}

func assertRowCount(t *testing.T, service *Service, ctx context.Context, query string, arg interface{}, want int) {
	t.Helper()
	var got int
	require.NoError(t, service.db.QueryRow(ctx, query, arg).Scan(&got))
	assert.Equal(t, want, got, "row count for %q", query)
}

func assertLogContains(t *testing.T, logs string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		assert.Contains(t, logs, fragment)
	}
}
