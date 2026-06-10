package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hiddenFlagForEvent(t *testing.T, rows []EventSummary, eventID string) bool {
	t.Helper()
	for _, row := range rows {
		if row.EventID == eventID {
			return row.Hidden
		}
	}
	t.Fatalf("event %s not found in list", eventID)
	return false
}

func TestHideEventTogglesCalendarVisibilityFlag(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	employee := Actor{ID: "E1001", Role: RoleEmployee}
	event := createFilterEvent(t, service, ctx, admin, filterEventInput{
		title:        "Hide Target",
		city:         "Taipei",
		capacityType: CapacityTypeLimited,
		startsAt:     time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC),
	})

	rows, err := service.ListEvents(ctx, employee, "")
	require.NoError(t, err)
	assert.False(t, hiddenFlagForEvent(t, rows, event.EventID), "event starts visible")

	require.NoError(t, service.HideEvent(ctx, employee, event.EventID))
	// A repeat hide is idempotent (composite primary key, ON CONFLICT DO NOTHING).
	require.NoError(t, service.HideEvent(ctx, employee, event.EventID))

	rows, err = service.ListEvents(ctx, employee, "")
	require.NoError(t, err)
	assert.True(t, hiddenFlagForEvent(t, rows, event.EventID), "hidden flag set after hide")

	// Hiding is per-employee: another employee's list is unaffected.
	otherRows, err := service.ListEvents(ctx, Actor{ID: "E1002", Role: RoleEmployee}, "")
	require.NoError(t, err)
	assert.False(t, hiddenFlagForEvent(t, otherRows, event.EventID), "hide must not leak across employees")

	require.NoError(t, service.UnhideEvent(ctx, employee, event.EventID))
	// Unhiding an event that is not hidden is a no-op.
	require.NoError(t, service.UnhideEvent(ctx, employee, event.EventID))

	rows, err = service.ListEvents(ctx, employee, "")
	require.NoError(t, err)
	assert.False(t, hiddenFlagForEvent(t, rows, event.EventID), "hidden flag cleared after unhide")
}

func TestHideEventRejectsUnknownEvent(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	err := service.HideEvent(ctx, Actor{ID: "E1001", Role: RoleEmployee}, "evt-does-not-exist")
	require.Error(t, err)
	assert.Equal(t, 404, ErrorStatus(err))
}

func TestHideEventRequiresEmployeeRole(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	err := service.HideEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, "evt-any")
	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))
}
