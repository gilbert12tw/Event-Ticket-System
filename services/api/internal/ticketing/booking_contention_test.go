package ticketing

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdvisoryContentionConcurrentBookingsDoNotOversellLastSeat(t *testing.T) {
	service, ctx := newAdvisoryContentionTest(t)
	event := createPublishedEvent(t, service, ctx, CreateEventRequest{Title: "Advisory Last Seat", Capacity: 1, Rule: engineeringRule(5)})

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
			res, err := service.Book(ctx, Actor{ID: employeeID, Role: RoleEmployee}, event.EventID,
				BookingRequest{EmployeeID: employeeID, IdempotencyKey: "advisory-race-" + employeeID})
			if err != nil {
				errs <- err
				return
			}
			results <- res
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	confirmed, waitlisted := countBookingStatuses(results)
	assert.Equal(t, 1, confirmed)
	assert.Equal(t, 1, waitlisted)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`, event.EventID, 1)
}

func TestAdvisoryContentionFullEventPreservesWaitlistOrderAndAudit(t *testing.T) {
	service, ctx := newAdvisoryContentionTest(t)
	event := createPublishedEvent(t, service, ctx, CreateEventRequest{Title: "Advisory Waitlist", Capacity: 1, Rule: engineeringRule(5)})
	insertBookingContentionEmployee(t, service, ctx, "E9001")

	confirmed, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID,
		BookingRequest{EmployeeID: "E1001", IdempotencyKey: "advisory-wait-confirm"})
	require.NoError(t, err)
	require.Equal(t, RegistrationConfirmed, confirmed.Registration.Status)

	firstWaitlist, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID,
		BookingRequest{EmployeeID: "E1002", IdempotencyKey: "advisory-wait-1"})
	require.NoError(t, err)
	require.Equal(t, RegistrationWaitlisted, firstWaitlist.Registration.Status)
	time.Sleep(10 * time.Millisecond)

	secondWaitlist, err := service.Book(ctx, Actor{ID: "E9001", Role: RoleEmployee}, event.EventID,
		BookingRequest{EmployeeID: "E9001", IdempotencyKey: "advisory-wait-2"})
	require.NoError(t, err)
	require.Equal(t, RegistrationWaitlisted, secondWaitlist.Registration.Status)

	rows, err := service.db.Query(ctx, `SELECT registration_id FROM registrations
		WHERE event_id = $1 AND status = 'waitlisted' ORDER BY created_at, registration_id`, event.EventID)
	require.NoError(t, err)
	defer rows.Close()
	var ordered []string
	for rows.Next() {
		var registrationID string
		require.NoError(t, rows.Scan(&registrationID))
		ordered = append(ordered, registrationID)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{firstWaitlist.Registration.RegistrationID, secondWaitlist.Registration.RegistrationID}, ordered)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'booking.waitlisted' AND metadata->>'event_id' = $1`, event.EventID, 2)
}

func TestAdvisoryContentionDuplicateEmployeeBookingDoesNotDuplicateSideEffects(t *testing.T) {
	service, ctx := newAdvisoryContentionTest(t)
	event := createPublishedEvent(t, service, ctx, CreateEventRequest{Title: "Advisory Duplicate", Capacity: 2, Rule: engineeringRule(5)})

	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID,
		BookingRequest{EmployeeID: "E1001", IdempotencyKey: "advisory-duplicate-1"})
	require.NoError(t, err)
	require.Equal(t, RegistrationConfirmed, first.Registration.Status)

	duplicate, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID,
		BookingRequest{EmployeeID: "E1001", IdempotencyKey: "advisory-duplicate-2"})
	require.NoError(t, err)

	assert.True(t, duplicate.Duplicate)
	assert.Equal(t, first.Registration.RegistrationID, duplicate.Registration.RegistrationID)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND employee_id = 'E1001'`, event.EventID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'booking.confirmed' AND metadata->>'event_id' = $1`, event.EventID, 1)
}

func newAdvisoryContentionTest(t *testing.T) (*Service, context.Context) {
	t.Helper()
	service, ctx := newSeededIntegrationTest(t)
	service.WithBookingContentionStrategy(BookingContentionStrategyAdvisory)
	return service, ctx
}

func countBookingStatuses(results <-chan BookingResponse) (int, int) {
	var confirmed int
	var waitlisted int
	for result := range results {
		switch result.Registration.Status {
		case RegistrationConfirmed:
			confirmed++
		case RegistrationWaitlisted:
			waitlisted++
		}
	}
	return confirmed, waitlisted
}

func insertBookingContentionEmployee(t *testing.T, service *Service, ctx context.Context, employeeID string) {
	t.Helper()
	_, err := service.db.Exec(ctx, `INSERT INTO employees (employee_id, full_name, department, site, job_grade, employment_status)
		VALUES ($1, $2, 'Engineering', 'Taipei HQ', 6, 'active') ON CONFLICT (employee_id) DO NOTHING`,
		employeeID, "Hot Path Employee")
	require.NoError(t, err)
}
