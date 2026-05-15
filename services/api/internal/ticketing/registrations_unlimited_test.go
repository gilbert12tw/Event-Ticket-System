package ticketing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnlimitedEventBookingConfirmsAndPersistsFamilyCount(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:        "Open House",
		Location:     "Taipei HQ",
		CapacityType: CapacityTypeUnlimited,
		Status:       EventStatusPublished,
		Rule:         RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	assert.True(t, event.AllowsFamily, "unlimited create should auto-enable allows_family")

	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "unl-1", FamilyCount: 3})
	require.NoError(t, err)
	assert.Equal(t, RegistrationConfirmed, first.Registration.Status)
	require.NotNil(t, first.Ticket, "unlimited booking should confirm and issue ticket")
	assert.Equal(t, 3, first.Registration.FamilyCount)
	assert.Equal(t, 0, first.RemainingCapacity)

	retry, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "unl-1", FamilyCount: 3})
	require.NoError(t, err)
	assert.Equal(t, first.Registration.RegistrationID, retry.Registration.RegistrationID)
	assert.Equal(t, 3, retry.Registration.FamilyCount)

	second, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "unl-2", FamilyCount: 0})
	require.NoError(t, err)
	assert.Equal(t, RegistrationConfirmed, second.Registration.Status)
}

func TestUnlimitedBookingRejectsFamilyCountOverCap(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:        "Open House",
		Location:     "Taipei HQ",
		CapacityType: CapacityTypeUnlimited,
		Status:       EventStatusPublished,
		Rule:         RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "unl-cap", FamilyCount: 11})
	require.Error(t, err, "family_count=11 should error")
	assert.Equal(t, 400, ErrorStatus(err))
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "unl-neg", FamilyCount: -1})
	require.Error(t, err, "family_count=-1 should error")
	assert.Equal(t, 400, ErrorStatus(err))
}

func TestLimitedBookingRejectsFamilyCount(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Limited Hike",
		Location: "Taipei HQ",
		Capacity: 5,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lim-fam", FamilyCount: 1})
	require.Error(t, err, "limited+family should error")
	assert.Equal(t, 400, ErrorStatus(err))

	ok, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lim-ok"})
	require.NoError(t, err)
	assert.Equal(t, 0, ok.Registration.FamilyCount)
	assert.Equal(t, RegistrationConfirmed, ok.Registration.Status)
}

func TestUnlimitedBookingCancelAndRebook(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:        "Open House",
		Location:     "Taipei HQ",
		CapacityType: CapacityTypeUnlimited,
		Status:       EventStatusPublished,
		Rule:         RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	booked, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "unl-cancel", FamilyCount: 2})
	require.NoError(t, err)
	_, err = service.CancelRegistration(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, booked.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "cancel-1", Reason: "change of plans"})
	require.NoError(t, err)
}
