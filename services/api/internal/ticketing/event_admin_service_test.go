package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEventUsesRoleScopedEmployeeContext(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Scoped Event",
		Location: "Taipei HQ",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "get-event-scope",
	})
	require.NoError(t, err)

	employeeSummary, err := service.GetEvent(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, "E2001")
	require.NoError(t, err)
	assert.Equal(t, RegistrationConfirmed, employeeSummary.CurrentUserStatus)
	assert.Equal(t, booking.Registration.RegistrationID, employeeSummary.CurrentUserRegistrationID)

	adminSummary, err := service.GetEvent(ctx, admin, event.EventID, "E1002")
	require.NoError(t, err)
	assert.Empty(t, adminSummary.CurrentUserStatus)

	_, err = service.GetEvent(ctx, Actor{ID: "payroll-1", Role: "payroll_admin"}, event.EventID, "")
	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))
}

func TestCreateEventAcceptsLotteryAllocationMode(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}

	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:          "Lottery Event",
		Location:       "Taipei HQ",
		Capacity:       2,
		Status:         EventStatusPublished,
		AllocationMode: AllocationModeLottery,
		Rule:           RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})

	require.NoError(t, err)
	assert.Equal(t, AllocationModeLottery, event.AllocationMode)
}

func TestUpdateEventPersistsEndTimeAndSyncsActiveTicketExpiry(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	service.WithClock(func() time.Time { return now })
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	startsAt := now.Add(72 * time.Hour)
	endsAt := startsAt.Add(2 * time.Hour)
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "End Time Event",
		Location:          "Taipei HQ",
		StartsAt:          startsAt,
		EndsAt:            endsAt,
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Capacity:          2,
		Status:            EventStatusPublished,
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	assert.WithinDuration(t, endsAt, event.EndsAt, 0)
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "end-time-ticket",
	})
	require.NoError(t, err)
	require.NotNil(t, booking.Ticket)
	assert.WithinDuration(t, endsAt, booking.Ticket.ExpiresAt, 0)
	nextEndsAt := startsAt.Add(4 * time.Hour)

	updated, err := service.UpdateEvent(ctx, admin, event.EventID, UpdateEventRequest{
		EndsAt: &nextEndsAt,
	})

	require.NoError(t, err)
	assert.WithinDuration(t, nextEndsAt, updated.EndsAt, 0)
	var versionEndsAt time.Time
	require.NoError(t, service.db.QueryRow(ctx, `SELECT ends_at FROM event_versions WHERE event_id = $1 AND version = $2`, event.EventID, updated.Version).Scan(&versionEndsAt))
	assert.WithinDuration(t, nextEndsAt, versionEndsAt, 0)
	var ticketExpiresAt time.Time
	require.NoError(t, service.db.QueryRow(ctx, `SELECT expires_at FROM tickets WHERE ticket_id = $1`, booking.Ticket.TicketID).Scan(&ticketExpiresAt))
	assert.WithinDuration(t, nextEndsAt, ticketExpiresAt, 0)
}

func TestCreateEventRejectsLotteryForUnlimitedCapacity(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}

	_, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:          "Invalid Lottery",
		Location:       "Taipei HQ",
		CapacityType:   CapacityTypeUnlimited,
		AllowsFamily:   true,
		AllocationMode: AllocationModeLottery,
		Rule:           RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})

	require.Error(t, err)
	assert.Equal(t, 400, ErrorStatus(err))
	assert.Equal(t, "lottery allocation requires limited capacity", ErrorMessage(err))
}

func TestUpdateEventRejectsAllocationModeChangeAfterRegistrations(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Allocation Locked",
		Location: "Taipei HQ",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "allocation-mode-lock",
	})
	require.NoError(t, err)
	next := AllocationModeLottery

	_, err = service.UpdateEvent(ctx, admin, event.EventID, UpdateEventRequest{AllocationMode: &next})

	require.Error(t, err)
	assert.Equal(t, 409, ErrorStatus(err))
	assert.Equal(t, "allocation_mode cannot change after registrations exist", ErrorMessage(err))
}
