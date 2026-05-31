package ticketing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookRejectsEventStateWindowAndFamilyConstraints(t *testing.T) {
	service, ctx := newSeededIntegrationTest(t)
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	employee := Actor{ID: "E1001", Role: RoleEmployee}

	_, err := service.Book(ctx, employee, "evt_missing", BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "missing-event-booking",
	})
	require.Error(t, err)
	assert.Equal(t, 404, ErrorStatus(err))

	draft, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Draft Booking Block",
		Capacity: 3,
		Status:   EventStatusDraft,
		Rule:     engineeringRule(0),
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, employee, draft.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "draft-booking",
	})
	require.Error(t, err)
	assert.Equal(t, 409, ErrorStatus(err))

	now := time.Now().UTC()
	closed, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Closed Booking Window",
		Capacity:          3,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(24 * time.Hour),
		RegistrationStart: now.Add(-4 * time.Hour),
		RegistrationClose: now.Add(-2 * time.Hour),
		Rule:              engineeringRule(0),
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, employee, closed.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "closed-window-booking",
	})
	require.Error(t, err)
	assert.Equal(t, 409, ErrorStatus(err))

	limitedFamily := createPublishedEvent(
		t,
		service,
		ctx,
		engineeringEventRequest("Limited Family Block", 3, 0),
	)
	_, err = service.Book(ctx, employee, limitedFamily.EventID, BookingRequest{
		EmployeeID:     "E1001",
		FamilyCount:    1,
		IdempotencyKey: "limited-family-booking",
	})
	require.Error(t, err)
	assert.Equal(t, 400, ErrorStatus(err))
}

func TestBookRejectsIncompleteProviderClaims(t *testing.T) {
	service, ctx := newSeededIntegrationTest(t)
	event := createPublishedEvent(
		t,
		service,
		ctx,
		engineeringEventRequest("Claims Required Booking", 3, 0),
	)

	_, err := service.Book(
		ctx,
		Actor{ID: "E1001", Role: RoleEmployee, Claims: &ProviderClaims{}},
		event.EventID,
		BookingRequest{
			EmployeeID:     "E1001",
			IdempotencyKey: "incomplete-claims-booking",
		},
	)

	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))
}

func TestGetEventSummaryDefaultEligibilityAndNotFound(t *testing.T) {
	service, ctx := newSeededIntegrationTest(t)
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event := createPublishedEvent(
		t,
		service,
		ctx,
		engineeringEventRequest("Summary Defaults", 3, 0),
	)

	summary, err := service.GetEventSummary(ctx, admin, event.EventID, "")
	require.NoError(t, err)
	assert.False(t, summary.Eligible)
	assert.Equal(t, "provider claims employee identity is required", summary.EligibilityReason)
	assert.False(t, summary.NoShowCooldown.Active)

	missingEmployee, err := service.GetEventSummary(ctx, admin, event.EventID, "E404")
	require.NoError(t, err)
	assert.False(t, missingEmployee.Eligible)
	assert.Equal(t, "employee not found", missingEmployee.EligibilityReason)

	missingClaims, err := service.GetEventSummary(
		ctx,
		Actor{ID: "E1001", Role: RoleEmployee, Claims: &ProviderClaims{}},
		event.EventID,
		"E1001",
	)
	require.NoError(t, err)
	assert.False(t, missingClaims.Eligible)
	assert.Equal(t, ErrMissingClaims.Error(), missingClaims.EligibilityReason)

	_, err = service.GetEventSummary(ctx, admin, "evt_missing", "")
	require.Error(t, err)
	assert.Equal(t, 404, ErrorStatus(err))
}
