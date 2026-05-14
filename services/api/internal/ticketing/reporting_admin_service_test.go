package ticketing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateReportExportStartsPendingWithOutboxAndAudit(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	hr := Actor{ID: "hr-1", Role: RoleHRAdmin}
	got, err := service.CreateReportExport(ctx, hr, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	assert.Equal(t, ReportExportStatusPending, got.Status)
	assert.NotEmpty(t, got.ObjectKey, "report export object key must be set")
	assert.True(t, got.CompletedAt.IsZero(), "pending report export should not be completed yet")

	var status string
	var completedAt sql.NullTime
	require.NoError(t, service.db.QueryRow(ctx, `SELECT status, completed_at FROM report_exports WHERE export_id = $1`, got.ExportID).Scan(&status, &completedAt))
	assert.Equal(t, ReportExportStatusPending, status)
	assert.False(t, completedAt.Valid, "stored report export completed_at should be null while pending")

	var outboxCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'report.export.requested' AND aggregate_id = $1`, got.ExportID).Scan(&outboxCount))
	assert.Equal(t, 1, outboxCount)

	var auditCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'report.export.requested' AND entity_id = $1`, got.ExportID).Scan(&auditCount))
	assert.Equal(t, 1, auditCount)
}

func TestGetReportExportRequiresHRAndReturnsPersistedStatus(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	hr := Actor{ID: "hr-1", Role: RoleHRAdmin}
	created, err := service.CreateReportExport(ctx, hr, ReportExportRequest{ReportType: "participation"})
	require.NoError(t, err)
	got, err := service.GetReportExport(ctx, hr, created.ExportID)
	require.NoError(t, err)
	assert.Equal(t, created.ExportID, got.ExportID)
	assert.Equal(t, ReportExportStatusPending, got.Status)
	assert.NotEmpty(t, got.ObjectKey)
	_, err = service.GetReportExport(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, created.ExportID)
	require.Error(t, err, "activity admin should not read report export status")
}

func TestRunLotteryAllocatesDeterministicWinnersAndLeavesRemainingWaitlisted(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Lottery Deterministic Test",
		Capacity: 3,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lottery-book-1"})
	require.NoError(t, err)

	extraEmployees := []Employee{
		{EmployeeID: "E3001", FullName: "Dora Wang", Department: "Engineering", Site: "Taipei", JobGrade: 6, EmploymentStatus: "active"},
		{EmployeeID: "E3002", FullName: "Eric Chen", Department: "Sales", Site: "Taipei", JobGrade: 4, EmploymentStatus: "active"},
	}
	for _, employee := range extraEmployees {
		_, err := service.db.Exec(ctx, `INSERT INTO employees (employee_id, full_name, department, site, job_grade, employment_status)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (employee_id) DO UPDATE SET full_name = EXCLUDED.full_name, department = EXCLUDED.department, site = EXCLUDED.site, job_grade = EXCLUDED.job_grade, employment_status = EXCLUDED.employment_status`,
			employee.EmployeeID, employee.FullName, employee.Department, employee.Site, employee.JobGrade, employee.EmploymentStatus)
		require.NoError(t, err)
	}

	waitlisted := []struct {
		registrationID string
		employeeID     string
		idempotencyKey string
	}{
		{registrationID: "reg_wait_a", employeeID: "E1002", idempotencyKey: "lottery-wait-1"},
		{registrationID: "reg_wait_b", employeeID: "E3001", idempotencyKey: "lottery-wait-2"},
		{registrationID: "reg_wait_c", employeeID: "E3002", idempotencyKey: "lottery-wait-3"},
	}
	for _, candidate := range waitlisted {
		_, err := service.db.Exec(ctx, `INSERT INTO registrations (registration_id, event_id, employee_id, status, idempotency_key)
			VALUES ($1,$2,$3,'waitlisted',$4)`, candidate.registrationID, event.EventID, candidate.employeeID, candidate.idempotencyKey)
		require.NoError(t, err)
	}

	var confirmedBefore int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`, event.EventID).Scan(&confirmedBefore))
	assert.Equal(t, 1, confirmedBefore)

	run, err := service.RunLottery(ctx, admin, event.EventID, LotteryRunRequest{Seed: "det-seed-1"})
	require.NoError(t, err)
	assert.Equal(t, 2, run.WinnerCount)

	var confirmedAfter int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`, event.EventID).Scan(&confirmedAfter))
	assert.Equal(t, 3, confirmedAfter)

	seed := "det-seed-1"
	expectedWinners := lotteryExpectedWinners(seed, []string{
		"reg_wait_a", "reg_wait_b", "reg_wait_c",
	}, 2)
	for _, candidate := range waitlisted {
		var status string
		require.NoError(t, service.db.QueryRow(ctx, `SELECT status FROM registrations WHERE registration_id = $1`, candidate.registrationID).Scan(&status))
		if expectedWinners[candidate.registrationID] {
			assert.Equal(t, RegistrationConfirmed, status, "registration %s", candidate.registrationID)
		} else {
			assert.Equal(t, RegistrationWaitlisted, status, "registration %s", candidate.registrationID)
		}
		var ticketCount int
		require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE registration_id = $1`, candidate.registrationID).Scan(&ticketCount))
		if expectedWinners[candidate.registrationID] {
			assert.Equal(t, 1, ticketCount, "winner %s ticket_count", candidate.registrationID)
		} else {
			assert.Equal(t, 0, ticketCount, "non-winner %s ticket_count", candidate.registrationID)
		}
	}

	var outboxCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'lottery.completed' AND aggregate_id = $1`, run.RunID).Scan(&outboxCount))
	assert.Equal(t, 1, outboxCount)
	var auditCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'lottery.completed' AND entity_type = 'event' AND entity_id = $1`, event.EventID).Scan(&auditCount))
	assert.Equal(t, 1, auditCount)
	var winnerResults, waitlistResults int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM lottery_results WHERE run_id = $1 AND result = 'winner'`, run.RunID).Scan(&winnerResults))
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM lottery_results WHERE run_id = $1 AND result = 'waitlisted'`, run.RunID).Scan(&waitlistResults))
	assert.Equal(t, 2, winnerResults)
	assert.Equal(t, 1, waitlistResults)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'ticket.issued' AND entity_id IN (SELECT ticket_id FROM lottery_results WHERE run_id = $1 AND result = 'winner')`, run.RunID, 2)
}

func TestRunLotteryReturnsExistingRunBySeed(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Lottery Idempotent Test",
		Capacity: 3,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lottery-idem-book-1"})
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `INSERT INTO registrations (registration_id, event_id, employee_id, status, idempotency_key)
		VALUES ('reg_wait_repeat_a', $1, 'E1002', 'waitlisted', 'lottery-idem-wait-1')`, event.EventID)
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `INSERT INTO registrations (registration_id, event_id, employee_id, status, idempotency_key)
		VALUES ('reg_wait_repeat_b', $1, 'E2001', 'waitlisted', 'lottery-idem-wait-2')`, event.EventID)
	require.NoError(t, err)

	first, err := service.RunLottery(ctx, admin, event.EventID, LotteryRunRequest{Seed: "repeat-seed"})
	require.NoError(t, err)
	second, err := service.RunLottery(ctx, admin, event.EventID, LotteryRunRequest{Seed: "repeat-seed"})
	require.NoError(t, err)
	assert.Equal(t, first.RunID, second.RunID)
	assert.Equal(t, first.WinnerCount, second.WinnerCount)

	var outboxCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'lottery.completed' AND aggregate_id = $1`, first.RunID).Scan(&outboxCount))
	assert.Equal(t, 1, outboxCount)
	var auditCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'lottery.completed' AND entity_type = 'event' AND entity_id = $1`, event.EventID).Scan(&auditCount))
	assert.Equal(t, 1, auditCount)
}

func lotteryExpectedWinners(seed string, registrationIDs []string, limit int) map[string]bool {
	type testLotteryCandidate struct {
		registrationID string
		drawSeed       string
	}
	candidates := make([]testLotteryCandidate, 0, len(registrationIDs))
	for _, registrationID := range registrationIDs {
		hash := sha256.Sum256([]byte(seed + "|" + registrationID))
		candidates = append(candidates, testLotteryCandidate{
			registrationID: registrationID,
			drawSeed:       hex.EncodeToString(hash[:]),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		switch cmp := strings.Compare(candidates[i].drawSeed, candidates[j].drawSeed); {
		case cmp < 0:
			return true
		case cmp > 0:
			return false
		default:
			return candidates[i].registrationID < candidates[j].registrationID
		}
	})

	if limit > len(candidates) {
		limit = len(candidates)
	}

	expected := make(map[string]bool, limit)
	for i := 0; i < limit; i++ {
		expected[candidates[i].registrationID] = true
	}
	return expected
}
