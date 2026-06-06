package ticketing

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// BenchmarkCreateEligibilityImpactReviews measures the impact-review insert path
// for varying numbers of affected registrations. The cost being optimised is the
// per-statement round-trip to Postgres, so this MUST run against a real database
// (TEST_DATABASE_URL); without it the harness skips. Each iteration runs inside a
// throwaway transaction that is rolled back, so the NOT EXISTS pending filter sees
// a clean slate every time and exactly N rows are inserted.
//
// Run: go test -bench=BenchmarkCreateEligibilityImpactReviews -run=^$ -benchmem ./internal/ticketing
func BenchmarkCreateEligibilityImpactReviews(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, n := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			service, cleanup := newIntegrationServiceWithLogger(b, logger)
			defer cleanup()
			ctx := context.Background()
			eventID := seedImpactReviewBenchData(b, service, ctx, n)
			// Rule that rejects every seeded employee, so all N rows become pending.
			rule := RuleInput{Department: "Legal", Site: "Nowhere", MinGrade: 99, EmploymentStatus: "active"}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				tx, err := service.db.Begin(ctx)
				require.NoError(b, err)
				b.StartTimer()

				count, err := service.createEligibilityImpactReviewsTx(ctx, tx, eventID, rule)

				b.StopTimer()
				if err != nil {
					_ = tx.Rollback(ctx)
					b.Fatal(err)
				}
				if count != n {
					_ = tx.Rollback(ctx)
					b.Fatalf("expected %d impact reviews, got %d", n, count)
				}
				require.NoError(b, tx.Rollback(ctx))
				b.StartTimer()
			}
		})
	}
}

// seedImpactReviewBenchData commits one event plus n employees and confirmed
// registrations, then returns the event id. Tickets are intentionally omitted
// (the join is a LEFT JOIN, ticket_id resolves to NULL).
func seedImpactReviewBenchData(tb testing.TB, service *Service, ctx context.Context, n int) string {
	tb.Helper()
	eventID := fmt.Sprintf("evt-bench-%d", time.Now().UnixNano())
	_, err := service.db.Exec(ctx, `INSERT INTO events
		(event_id, title, starts_at, registration_start, registration_close, capacity_type, capacity, status, created_by)
		VALUES ($1, 'Bench Event', now() + interval '7 days', now() - interval '1 day', now() + interval '6 days', 'limited', $2, 'published', 'admin-bench')`,
		eventID, n+1)
	require.NoError(tb, err)

	batch := &pgx.Batch{}
	for i := 0; i < n; i++ {
		employeeID := fmt.Sprintf("%s-emp-%d", eventID, i)
		batch.Queue(`INSERT INTO employees
			(employee_id, full_name, department, site, job_grade, employment_status)
			VALUES ($1, 'Bench Employee', 'Engineering', 'Taipei HQ', 5, 'active')`, employeeID)
		batch.Queue(`INSERT INTO registrations
			(registration_id, event_id, employee_id, status, idempotency_key)
			VALUES ($1, $2, $3, 'confirmed', $4)`,
			fmt.Sprintf("%s-reg-%d", eventID, i), eventID, employeeID, fmt.Sprintf("%s-idem-%d", eventID, i))
	}
	results := service.db.SendBatch(ctx, batch)
	for i := 0; i < n*2; i++ {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			require.NoError(tb, err)
		}
	}
	require.NoError(tb, results.Close())
	return eventID
}
