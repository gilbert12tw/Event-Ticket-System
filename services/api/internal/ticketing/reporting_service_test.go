package ticketing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportsIncludeAggregateAttendanceAndCityDistribution(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	hr := Actor{ID: "hr-1", Role: RoleHRAdmin}
	limited, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:     "Limited Report Aggregate",
		EventCity: "Taipei",
		Capacity:  1,
		Status:    EventStatusPublished,
		Rule:      RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	unlimited, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:        "Unlimited Report Aggregate",
		EventCity:    "Hsinchu",
		CapacityType: CapacityTypeUnlimited,
		Status:       EventStatusPublished,
		Rule:         RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, limited.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "report-limited-confirmed"})
	require.NoError(t, err)
	waitlisted, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, limited.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "report-limited-waitlisted"})
	require.NoError(t, err)
	assert.Equal(t, RegistrationWaitlisted, waitlisted.Registration.Status)

	confirmed, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, unlimited.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "report-unlimited-confirmed", FamilyCount: 2})
	require.NoError(t, err)
	cancelled, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, unlimited.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "report-unlimited-cancelled", FamilyCount: 4})
	require.NoError(t, err)
	_, err = service.CancelRegistration(ctx, Actor{ID: "E1002", Role: RoleEmployee}, unlimited.EventID, cancelled.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "report-unlimited-cancel", Reason: "schedule conflict"})
	require.NoError(t, err)

	reports, err := service.Reports(ctx, hr)
	require.NoError(t, err)
	limitedReport := requireReportRow(t, reports.Rows, limited.EventID)
	assert.Equal(t, 1, limitedReport.ConfirmedCount)
	assert.Equal(t, 1, limitedReport.WaitlistCount)
	assert.Equal(t, 1, limitedReport.EmployeeCount)
	assert.Equal(t, 0, limitedReport.FamilyCount)
	assert.Equal(t, 1, limitedReport.TotalAttendeeCount)
	assert.Equal(t, map[string]int{"Taipei": 1}, limitedReport.CityDistribution)

	unlimitedReport := requireReportRow(t, reports.Rows, unlimited.EventID)
	assert.Equal(t, 1, unlimitedReport.ConfirmedCount)
	assert.Equal(t, 1, unlimitedReport.EmployeeCount)
	assert.Equal(t, confirmed.Registration.FamilyCount, unlimitedReport.FamilyCount)
	assert.Equal(t, 3, unlimitedReport.TotalAttendeeCount)
	assert.Equal(t, map[string]int{"Hsinchu": 3}, unlimitedReport.CityDistribution)
}

func TestReportsUseUnknownCityWhenEventCityIsEmpty(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	hr := Actor{ID: "hr-1", Role: RoleHRAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Unknown City Aggregate",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	reports, err := service.Reports(ctx, hr)
	require.NoError(t, err)
	row := requireReportRow(t, reports.Rows, event.EventID)
	assert.Equal(t, map[string]int{"unknown": 0}, row.CityDistribution)
}

func requireReportRow(t *testing.T, reports []ReportRow, eventID string) ReportRow {
	t.Helper()
	for _, row := range reports {
		if row.EventID == eventID {
			return row
		}
	}
	require.Failf(t, "report row not found", "event_id=%s", eventID)
	return ReportRow{}
}
