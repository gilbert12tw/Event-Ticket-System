package ticketing

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunHRSyncCreatesBatchAndImpactReviews(t *testing.T) {
	service, cleanup := newIntegrationServiceWithLogger(t, slog.Default())
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "HR Sync Event",
		Capacity: 10,
		Status:   EventStatusPublished,
		Rule: RuleInput{
			Department:       "*",
			Site:             "*",
			MinGrade:         0,
			EmploymentStatus: "active",
		},
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "sync-e1001"})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "sync-e1002"})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E2001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E2001", IdempotencyKey: "sync-e2001"})
	require.NoError(t, err)

	_, err = service.db.Exec(ctx, `
		UPDATE eligibility_rules
		SET department = $1, site = $2, min_grade = $3, employment_status = $4
		WHERE event_id = $5`, "Engineering", "Taipei HQ", 6, "active", event.EventID)
	require.NoError(t, err)

	batch, err := service.RunHRSync(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, HRSyncRequest{})
	require.NoError(t, err)
	assert.Equal(t, "manual", batch.Source)
	assert.Equal(t, "applied", batch.Status)
	assert.Equal(t, 2, batch.EmployeeCount)

	var persistedStatus string
	var persistedCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT status, employee_count FROM hr_sync_batches WHERE batch_id = $1`, batch.BatchID).Scan(&persistedStatus, &persistedCount))
	assert.Equal(t, "applied", persistedStatus)
	assert.Equal(t, 2, persistedCount)

	assertRowCount(t, service, ctx, `SELECT count(*) FROM eligibility_impact_reviews WHERE event_id = $1 AND status = 'pending'`, event.EventID, 2)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'eligibility.impact_review.created' AND aggregate_id = $1`, event.EventID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'hr_sync.completed' AND aggregate_id = $1`, batch.BatchID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'eligibility_impact.created' AND entity_id = $1`, event.EventID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'hr_sync.completed' AND entity_id = $1`, batch.BatchID, 1)
}

func TestRunHRSyncRequiresHRAdmin(t *testing.T) {
	service, cleanup := newIntegrationServiceWithLogger(t, slog.Default())
	defer cleanup()

	_, err := service.RunHRSync(context.Background(), Actor{ID: "emp-1", Role: RoleEmployee}, HRSyncRequest{})
	require.Error(t, err)
	assert.Equal(t, 403, ErrorStatus(err))
}
