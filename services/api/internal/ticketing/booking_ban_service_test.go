package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookingBanCreatedAfterConfirmedCancellation(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Ban Target",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	employee := Actor{ID: "E1001", Role: RoleEmployee}
	booking, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "ban-book"})
	require.NoError(t, err)
	require.Equal(t, RegistrationConfirmed, booking.Registration.Status)

	_, err = service.CancelMyRegistration(ctx, employee, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "ban-cancel", Reason: "sick"})
	require.NoError(t, err)

	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1 AND lifted_at IS NULL`, "E1001", 1)
}

func TestBookingBanNotCreatedAfterWaitlistCancellation(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Waitlist Ban Target",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "w-book-1"})
	require.NoError(t, err)

	employee2 := Actor{ID: "E1002", Role: RoleEmployee}
	booking2, err := service.Book(ctx, employee2, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "w-book-2"})
	require.NoError(t, err)
	require.Equal(t, RegistrationWaitlisted, booking2.Registration.Status)

	_, err = service.CancelMyRegistration(ctx, employee2, booking2.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "w-cancel", Reason: "sick"})
	require.NoError(t, err)

	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1 AND lifted_at IS NULL`, "E1002", 0)
}

func TestBannedEmployeeCannotRebook(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Banned Rebook",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	employee := Actor{ID: "E1001", Role: RoleEmployee}
	booking, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "b-book"})
	require.NoError(t, err)

	_, err = service.CancelMyRegistration(ctx, employee, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "b-cancel", Reason: "sick"})
	require.NoError(t, err)

	_, err = service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "b-rebook"})
	require.Error(t, err)
	assert.Equal(t, 422, ErrorStatus(err))
	assert.Equal(t, "BOOKING_BANNED", ErrorCode(err))
}

func TestBannedEmployeeCannotJoinWaitlist(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Banned Waitlist",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	employee := Actor{ID: "E1001", Role: RoleEmployee}
	booking, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "bw-book"})
	require.NoError(t, err)

	_, err = service.CancelMyRegistration(ctx, employee, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "bw-cancel", Reason: "sick"})
	require.NoError(t, err)

	// Another employee takes the spot.
	_, err = service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "bw-book-2"})
	require.NoError(t, err)

	// Banned employee tries to book (which would normally waitlist)
	_, err = service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "bw-rebook"})
	require.Error(t, err)
	assert.Equal(t, 422, ErrorStatus(err))
	assert.Equal(t, "BOOKING_BANNED", ErrorCode(err))
}

// Regression: the event list must surface a banned employee's status as
// cancelled so the UI disables the "加入候補" action, matching the detail page.
func TestBannedEmployeeListShowsCancelledStatus(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Banned List Status",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	employee := Actor{ID: "E1001", Role: RoleEmployee}
	booking, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "bl-book"})
	require.NoError(t, err)

	_, err = service.CancelMyRegistration(ctx, employee, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "bl-cancel", Reason: "sick"})
	require.NoError(t, err)

	// Another employee takes the spot so the event is full again.
	_, err = service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "bl-book-2"})
	require.NoError(t, err)

	summaries, err := service.ListEvents(ctx, employee, "E1001")
	require.NoError(t, err)

	var found bool
	for _, s := range summaries {
		if s.EventID == event.EventID {
			found = true
			assert.Equal(t, RegistrationCancelled, s.CurrentUserStatus)
		}
	}
	require.True(t, found, "event should appear in published list")
}
