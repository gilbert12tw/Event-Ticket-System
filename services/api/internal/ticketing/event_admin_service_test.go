package ticketing

import (
	"context"
	"testing"

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
