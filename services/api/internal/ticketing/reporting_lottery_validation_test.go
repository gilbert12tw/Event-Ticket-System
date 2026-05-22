package ticketing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunLotteryRejectsFCFSEventWithoutSideEffects(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event := createLotteryValidationEvent(t, service, ctx, admin)

	assertLotteryRejectedWithoutAllocation(t, service, ctx, event.EventID, "fcfs-seed", "lottery allocation is not enabled for this event")
}

func TestRunLotteryRejectsClosedEventWithoutSideEffects(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event := createLotteryValidationEvent(t, service, ctx, admin)
	setLotteryAllocationMode(t, service, ctx, event.EventID)
	_, err := service.db.Exec(ctx, `UPDATE events SET status = $1 WHERE event_id = $2`, EventStatusClosed, event.EventID)
	require.NoError(t, err)

	assertLotteryRejectedWithoutAllocation(t, service, ctx, event.EventID, "closed-seed", "lottery can only run for published events")
}

func createLotteryValidationEvent(t *testing.T, service *Service, ctx context.Context, admin Actor) EventSummary {
	t.Helper()
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Lottery Guard",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lottery-guard-confirmed"})
	require.NoError(t, err)
	waitlisted, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "lottery-guard-waitlisted"})
	require.NoError(t, err)
	assert.Equal(t, RegistrationWaitlisted, waitlisted.Registration.Status)
	return event
}

func assertLotteryRejectedWithoutAllocation(t *testing.T, service *Service, ctx context.Context, eventID string, seed string, message string) {
	t.Helper()
	before := lotterySideEffectCounts(t, service, ctx, eventID)

	_, err := service.RunLottery(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, eventID, LotteryRunRequest{Seed: seed})

	require.Error(t, err)
	assert.Equal(t, 409, ErrorStatus(err))
	assert.Equal(t, message, ErrorMessage(err))
	assert.Equal(t, before, lotterySideEffectCounts(t, service, ctx, eventID))
}

func lotterySideEffectCounts(t *testing.T, service *Service, ctx context.Context, eventID string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	queries := map[string]string{
		"confirmed":       `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`,
		"waitlisted":      `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'waitlisted'`,
		"tickets":         `SELECT count(*) FROM tickets WHERE event_id = $1`,
		"lottery_runs":    `SELECT count(*) FROM lottery_runs WHERE event_id = $1`,
		"lottery_results": `SELECT count(*) FROM lottery_results WHERE event_id = $1`,
		"lottery_audits":  `SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'lottery.completed'`,
		"ticket_audits":   `SELECT count(*) FROM audit_logs WHERE action = 'ticket.issued' AND entity_id IN (SELECT ticket_id FROM tickets WHERE event_id = $1)`,
		"lottery_outbox":  `SELECT count(*) FROM outbox_events WHERE event_type = 'lottery.completed' AND (payload->>'event_id') = $1`,
		"ticket_outbox":   `SELECT count(*) FROM outbox_events WHERE event_type = 'ticket.issued' AND (payload->>'event_id') = $1`,
		"redeemed_outbox": `SELECT count(*) FROM outbox_events WHERE event_type = 'ticket.redeemed' AND (payload->>'event_id') = $1`,
	}
	for key, query := range queries {
		var count int
		require.NoError(t, service.db.QueryRow(ctx, query, eventID).Scan(&count), key)
		counts[key] = count
	}
	return counts
}
