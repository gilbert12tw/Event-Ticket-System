package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListEventsAppliesOpenAPIFilters(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	employee := Actor{ID: "E1001", Role: RoleEmployee}

	later := createFilterEvent(t, service, ctx, admin, filterEventInput{
		title:        "Taipei Limited Later",
		city:         "Taipei",
		capacityType: CapacityTypeLimited,
		startsAt:     time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC),
	})
	early := createFilterEvent(t, service, ctx, admin, filterEventInput{
		title:        "Taipei Limited Early",
		city:         "Taipei",
		capacityType: CapacityTypeLimited,
		startsAt:     time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC),
	})
	createFilterEvent(t, service, ctx, admin, filterEventInput{
		title:        "Taipei Unlimited",
		city:         "Taipei",
		capacityType: CapacityTypeUnlimited,
		startsAt:     time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC),
	})
	createFilterEvent(t, service, ctx, admin, filterEventInput{
		title:        "Tainan Limited",
		city:         "Tainan",
		capacityType: CapacityTypeLimited,
		startsAt:     time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
	})
	closed := createFilterEvent(t, service, ctx, admin, filterEventInput{
		title:        "Taipei Closed",
		city:         "Taipei",
		capacityType: CapacityTypeLimited,
		startsAt:     time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC),
	})
	_, err := service.ChangeEventState(ctx, admin, closed.EventID, ChangeEventStateRequest{
		Status: EventStatusClosed,
		Reason: "filter test",
	})
	require.NoError(t, err)

	rows, err := service.ListEvents(ctx, employee, "", EventListQuery{
		CapacityType: CapacityTypeLimited,
		City:         "Taipei",
	})

	require.NoError(t, err)
	assert.Equal(t, []string{early.EventID, later.EventID}, eventIDs(rows))

	closedRows, err := service.ListEvents(ctx, employee, "", EventListQuery{
		City:   "Taipei",
		Status: EventStatusClosed,
	})

	require.NoError(t, err)
	assert.Equal(t, []string{closed.EventID}, eventIDs(closedRows))

	_, err = service.ListEvents(ctx, employee, "", EventListQuery{Status: "retired"})
	require.Error(t, err)
	assert.Equal(t, 400, ErrorStatus(err))
}

type filterEventInput struct {
	title        string
	city         string
	capacityType string
	startsAt     time.Time
}

func createFilterEvent(t *testing.T, service *Service, ctx context.Context, admin Actor, input filterEventInput) EventSummary {
	t.Helper()
	req := CreateEventRequest{
		Title:             input.title,
		Description:       input.title,
		Location:          input.city + " HQ",
		EventCity:         input.city,
		EventSite:         input.city + " HQ",
		StartsAt:          input.startsAt,
		RegistrationStart: input.startsAt.Add(-48 * time.Hour),
		RegistrationClose: input.startsAt.Add(-24 * time.Hour),
		CapacityType:      input.capacityType,
		Capacity:          12,
		Status:            EventStatusPublished,
		Rule:              RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	}
	if input.capacityType == CapacityTypeUnlimited {
		req.Capacity = 0
		req.AllowsFamily = true
	}
	event, err := service.CreateEvent(ctx, admin, req)
	require.NoError(t, err)
	return event
}

func eventIDs(rows []EventSummary) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.EventID)
	}
	return ids
}
