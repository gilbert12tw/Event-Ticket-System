package ticketing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewEligibilityReturnsMatchCountsAndZeroMatch(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event := createPreviewEligibilityEvent(t, service, ctx)

	matching, err := service.PreviewEligibility(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, event.EventID, EligibilityPreviewRequest{
		Rule: RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	assert.Equal(t, event.EventID, matching.EventID)
	assert.Equal(t, 2, matching.MatchCount)
	assert.False(t, matching.ZeroMatch)

	zero, err := service.PreviewEligibility(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, event.EventID, EligibilityPreviewRequest{
		Rule: RuleInput{Department: "Legal", Site: "Nowhere", MinGrade: 99, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	assert.Equal(t, event.EventID, zero.EventID)
	assert.Equal(t, 0, zero.MatchCount)
	assert.True(t, zero.ZeroMatch)
}

func TestPreviewEligibilityRequiresActivityAdmin(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event := createPreviewEligibilityEvent(t, service, ctx)

	_, err := service.PreviewEligibility(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, event.EventID, EligibilityPreviewRequest{
		Rule: RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})

	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))
}

func createPreviewEligibilityEvent(t *testing.T, service *Service, ctx context.Context) EventSummary {
	t.Helper()
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Eligibility Preview",
		Capacity: 10,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	return event
}
