package ticketing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PH2-45 rebuild tests for the export aggregate columns. Seeding helpers
// (seedRebuildEvent, seedRebuildEmployee, seedRegistration, ...) live in
// rebuild_projection_service_test.go.

func seedRegistrationWithFamily(t *testing.T, s *Service, ctx context.Context, regID, eventID, employeeID, status string, familyCount int) {
	t.Helper()
	_, err := s.db.Exec(ctx, `
		INSERT INTO registrations (registration_id, event_id, employee_id, status, idempotency_key, family_count)
		VALUES ($1, $2, $3, $4, $1, $5)`, regID, eventID, employeeID, status, familyCount)
	require.NoError(t, err)
}

func seedTicket(t *testing.T, s *Service, ctx context.Context, ticketID, regID, eventID, employeeID string) {
	t.Helper()
	_, err := s.db.Exec(ctx, `
		INSERT INTO tickets (ticket_id, registration_id, event_id, employee_id, status, signed_token_hash)
		VALUES ($1, $2, $3, $4, 'active', $1)`, ticketID, regID, eventID, employeeID)
	require.NoError(t, err)
}

func seedCheckin(t *testing.T, s *Service, ctx context.Context, checkinID, ticketID string) {
	t.Helper()
	seedCheckinWithStatus(t, s, ctx, checkinID, ticketID, "accepted")
}

func seedCheckinWithStatus(t *testing.T, s *Service, ctx context.Context, checkinID, ticketID, status string) {
	t.Helper()
	_, err := s.db.Exec(ctx, `
		INSERT INTO checkin_records (checkin_id, ticket_id, staff_id, device_id, status)
		VALUES ($1, $2, 'staff-1', 'device-1', $3)`, checkinID, ticketID, status)
	require.NoError(t, err)
}

// Rebuild populates employee/family/ticket/checkin counts from OLTP with the
// same semantics as the Phase 1 Reports() query: confirmed-only employees and
// family sums, DISTINCT tickets, check-ins joined through tickets.
func TestRebuildProjection_ExportAggregateColumns(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	seedRebuildEvent(t, service, ctx, "evtA")
	seedRebuildEmployee(t, service, ctx, "ENG1", "Engineering")
	seedRebuildEmployee(t, service, ctx, "ENG2", "Engineering")
	seedRebuildEmployee(t, service, ctx, "HR1", "HR")
	seedRegistrationWithFamily(t, service, ctx, "r1", "evtA", "ENG1", "confirmed", 2)
	seedRegistrationWithFamily(t, service, ctx, "r2", "evtA", "ENG2", "confirmed", 1)
	// Waitlisted family members must not count toward family_count.
	seedRegistrationWithFamily(t, service, ctx, "r3", "evtA", "HR1", "waitlisted", 4)
	seedTicket(t, service, ctx, "t1", "r1", "evtA", "ENG1")
	seedTicket(t, service, ctx, "t2", "r2", "evtA", "ENG2")
	seedCheckin(t, service, ctx, "c1", "t1")

	_, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)

	row := readSummary(t, service, ctx, "evtA")
	assert.Equal(t, 2, row.ConfirmedCount)
	assert.Equal(t, 1, row.WaitlistCount)
	assert.Equal(t, 2, row.EmployeeCount)
	assert.Equal(t, 3, row.FamilyCount, "family_count sums confirmed registrations only")
	assert.Equal(t, 2, row.TicketCount)
	assert.Equal(t, 1, row.CheckinCount)
}

// employee_count counts confirmed registrations only: a cancelled registration
// for the same employee does not inflate it.
func TestRebuildProjection_EmployeeCountConfirmedOnly(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	seedRebuildEvent(t, service, ctx, "evtA")
	seedRebuildEmployee(t, service, ctx, "ENG1", "Engineering")
	seedRegistration(t, service, ctx, "r1-cancelled", "evtA", "ENG1", "cancelled")
	seedRegistration(t, service, ctx, "r2-confirmed", "evtA", "ENG1", "confirmed")

	_, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)

	row := readSummary(t, service, ctx, "evtA")
	assert.Equal(t, 1, row.ConfirmedCount)
	assert.Equal(t, 1, row.CancelledCount)
	assert.Equal(t, 1, row.EmployeeCount, "cancelled registration must not inflate employee_count")
}

// checkin_count counts accepted check-ins only. The incremental worker adds
// +1 per checkin.completed (accepted) event, so a rebuild over data containing
// status='conflict' rows must not diverge from the incrementally-built count.
func TestRebuildProjection_CheckinCountAcceptedOnly(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	seedRebuildEvent(t, service, ctx, "evtA")
	seedRebuildEmployee(t, service, ctx, "ENG1", "Engineering")
	seedRebuildEmployee(t, service, ctx, "ENG2", "Engineering")
	seedRegistration(t, service, ctx, "r1", "evtA", "ENG1", "confirmed")
	seedRegistration(t, service, ctx, "r2", "evtA", "ENG2", "confirmed")
	seedTicket(t, service, ctx, "t1", "r1", "evtA", "ENG1")
	seedTicket(t, service, ctx, "t2", "r2", "evtA", "ENG2")
	seedCheckin(t, service, ctx, "c1", "t1")
	seedCheckinWithStatus(t, service, ctx, "c2", "t2", "conflict")

	_, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)

	row := readSummary(t, service, ctx, "evtA")
	assert.Equal(t, 1, row.CheckinCount, "conflict check-in must not count toward checkin_count")
}

// PH2-45 / WS5-AC-4: rebuild drives from events, so a zero-activity event gets
// a genuine zero row — a missing projection row then unambiguously means
// "never projected" (pending_projection), not "no bookings yet".
func TestRebuildProjection_ZeroActivityEventGetsZeroRow(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	seedRebuildEvent(t, service, ctx, "evtQuiet")

	result, err := service.RebuildProjection(ctx, systemAdmin, RebuildOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, result.RowsInserted)

	require.True(t, summaryRowExists(t, service, ctx, "evtQuiet"))
	row := readSummary(t, service, ctx, "evtQuiet")
	assert.Zero(t, row.ConfirmedCount)
	assert.Zero(t, row.EmployeeCount)
	assert.Zero(t, row.FamilyCount)
	assert.Zero(t, row.TicketCount)
	assert.Zero(t, row.CheckinCount)
}
