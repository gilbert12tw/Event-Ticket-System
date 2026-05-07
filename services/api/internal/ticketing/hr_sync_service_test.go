package ticketing

import (
	"context"
	"log/slog"
	"testing"
)

func TestRunHRSyncCreatesBatchAndImpactReviews(t *testing.T) {
	service, cleanup := newIntegrationServiceWithLogger(t, slog.Default())
	defer cleanup()

	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

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
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "sync-e1001"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "sync-e1002"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Book(ctx, Actor{ID: "E2001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E2001", IdempotencyKey: "sync-e2001"}); err != nil {
		t.Fatal(err)
	}

	if _, err := service.db.Exec(ctx, `
		UPDATE eligibility_rules
		SET department = $1, site = $2, min_grade = $3, employment_status = $4
		WHERE event_id = $5`, "Engineering", "Taipei", 6, "active", event.EventID); err != nil {
		t.Fatal(err)
	}

	batch, err := service.RunHRSync(ctx, Actor{ID: "hr-1", Role: RoleHRAdmin}, HRSyncRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Source != "manual" {
		t.Fatalf("batch source = %q, want manual", batch.Source)
	}
	if batch.Status != "applied" {
		t.Fatalf("batch status = %q, want applied", batch.Status)
	}
	if batch.EmployeeCount != 2 {
		t.Fatalf("batch employee_count = %d, want 2", batch.EmployeeCount)
	}

	var persistedStatus string
	var persistedCount int
	if err := service.db.QueryRow(ctx, `SELECT status, employee_count FROM hr_sync_batches WHERE batch_id = $1`, batch.BatchID).Scan(&persistedStatus, &persistedCount); err != nil {
		t.Fatal(err)
	}
	if persistedStatus != "applied" || persistedCount != 2 {
		t.Fatalf("batch record status=%q count=%d, want status=%q count=%d", persistedStatus, persistedCount, "applied", 2)
	}

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
	if err == nil || ErrorStatus(err) != 403 {
		t.Fatalf("expected role check failure, got %v", err)
	}
}
