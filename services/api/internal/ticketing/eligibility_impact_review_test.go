package ticketing

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveEligibilityImpactReviewRedactsEmployeeAndWritesAudit(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	review := createPendingEligibilityImpactReview(t, service, ctx)

	resolved, err := service.ResolveEligibilityImpactReview(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, review.ReviewID, ResolveImpactReviewRequest{
		Reason: "HR confirmed employee is no longer eligible.",
	})

	require.NoError(t, err)
	assert.Equal(t, review.ReviewID, resolved.ReviewID)
	assert.Equal(t, "resolved", resolved.Status)
	assert.Equal(t, "E100****", resolved.EmployeeRef)
	assert.Equal(t, "HR confirmed employee is no longer eligible.", resolved.Reason)
	assert.False(t, resolved.ResolvedAt.IsZero())

	rawReview, err := json.Marshal(resolved)
	require.NoError(t, err)
	assert.Contains(t, string(rawReview), `"employee_ref":"E100****"`)
	assert.NotContains(t, string(rawReview), `"employee_id"`)
	assert.NotContains(t, string(rawReview), `"E1001"`)

	assertRowCount(t, service, ctx, `SELECT count(*) FROM eligibility_impact_reviews WHERE review_id = $1 AND status = 'resolved'`, review.ReviewID, 1)
	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs WHERE action = 'eligibility_impact.resolved' AND entity_id = $1`, review.ReviewID)
	assert.Equal(t, review.EventID, audit["event_id"])
	assert.Equal(t, "E100****", audit["employee_ref"])
	assertNoSensitiveJSONValues(t, audit, "E1001")
}

func TestResolveEligibilityImpactReviewReturnsNotFoundForMissingReview(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	_, err := service.ResolveEligibilityImpactReview(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, "rev_missing", ResolveImpactReviewRequest{
		Reason: "nothing to resolve",
	})

	require.Error(t, err)
	assert.Equal(t, 404, ErrorStatus(err))
	assert.Equal(t, "impact review not found", ErrorMessage(err))
}

func createPendingEligibilityImpactReview(t *testing.T, service *Service, ctx context.Context) EligibilityImpactReview {
	t.Helper()
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Eligibility Resolve",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "impact-resolve-booking",
	})
	require.NoError(t, err)
	_, err = service.UpdateEligibility(ctx, admin, event.EventID, UpdateEligibilityRequest{
		Rule: RuleInput{Department: "Sales", Site: "Taipei HQ", MinGrade: 4, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	reviews, err := service.EligibilityImpactReviews(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin})
	require.NoError(t, err)
	require.NotEmpty(t, reviews)
	return reviews[0]
}
