package ticketing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

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
	assert.Equal(t, ReportExportTypeParticipation, got.ReportType)
	assert.Equal(t, ReportExportFormatCSV, got.Format)
	assert.Equal(t, ReportExportStatusPending, got.Status)
	assert.NotEmpty(t, got.ObjectKey, "report export object key must be set")
	assert.Nil(t, got.CompletedAt, "pending report export should not be completed yet")

	var status string
	var completedAt sql.NullTime
	require.NoError(t, service.db.QueryRow(ctx, `SELECT status, completed_at FROM report_exports WHERE export_id = $1`, got.ExportID).Scan(&status, &completedAt))
	assert.Equal(t, ReportExportStatusPending, status)
	assert.False(t, completedAt.Valid, "stored report export completed_at should be null while pending")
	assertPendingReportExportOmitsCompletedAt(t, got)

	var (
		schemaVersion  int
		idempotencyKey string
		partitionKey   string
		payloadText    string
	)
	require.NoError(t, service.db.QueryRow(ctx, `SELECT schema_version, COALESCE(idempotency_key, ''), COALESCE(partition_key, ''), payload::text
		FROM outbox_events WHERE event_type = $1 AND aggregate_id = $2`,
		outboxEventReportExportRequestedV2, got.ExportID).Scan(&schemaVersion, &idempotencyKey, &partitionKey, &payloadText))
	assert.Equal(t, 2, schemaVersion)
	assert.Equal(t, "report.export.requested:"+got.ExportID, idempotencyKey)
	assert.Equal(t, got.ExportID, partitionKey)
	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(payloadText), &envelope))
	assert.Equal(t, outboxEventReportExportRequestedV2, envelope["event_type"])
	assert.Equal(t, idempotencyKey, envelope["idempotency_key"])
	assert.Equal(t, partitionKey, envelope["partition_key"])
	payload, ok := envelope["payload"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, got.ExportID, payload["export_id"])
	assert.Equal(t, ReportExportTypeParticipation, payload["report_type"])
	assert.Equal(t, hr.ID, payload["requested_by_employee_id"])
	requestedAt, ok := payload["requested_at"].(string)
	require.True(t, ok)
	_, err = time.Parse(time.RFC3339Nano, requestedAt)
	require.NoError(t, err)
	filters, ok := payload["filters"].(map[string]interface{})
	require.True(t, ok)
	assert.Empty(t, filters)
	assert.NotContains(t, payload, "object_key")
	assert.NotContains(t, payload, "requested_by")

	var auditCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'report.export.requested' AND entity_id = $1`, got.ExportID).Scan(&auditCount))
	assert.Equal(t, 1, auditCount)
}

func TestCreateReportExportRejectsUnsupportedContracts(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	hr := Actor{ID: "hr-1", Role: RoleHRAdmin}

	tests := []struct {
		name string
		req  ReportExportRequest
	}{
		{name: "missing report type"},
		{name: "unsupported report type", req: ReportExportRequest{ReportType: "tickets"}},
		{name: "unsupported format", req: ReportExportRequest{ReportType: "participation", Format: "json"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.CreateReportExport(ctx, hr, tt.req)

			require.Error(t, err)
			assert.Equal(t, 400, ErrorStatus(err))
		})
	}
	assertReportExportSideEffects(t, service, ctx, 0)
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
	assert.Equal(t, ReportExportFormatCSV, got.Format)
	assert.Equal(t, ReportExportStatusPending, got.Status)
	assert.NotEmpty(t, got.ObjectKey)
	assert.Nil(t, got.CompletedAt)
	assertPendingReportExportOmitsCompletedAt(t, got)
	_, err = service.GetReportExport(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, created.ExportID)
	require.Error(t, err, "activity admin should not read report export status")
}

func assertPendingReportExportOmitsCompletedAt(t *testing.T, export ReportExport) {
	t.Helper()
	payload, err := json.Marshal(export)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "completed_at")
}

func assertReportExportSideEffects(t *testing.T, service *Service, ctx context.Context, want int) {
	t.Helper()
	for _, query := range []string{
		`SELECT count(*) FROM report_exports`,
		`SELECT count(*) FROM outbox_events WHERE event_type = 'report.export.requested.v2'`,
		`SELECT count(*) FROM audit_logs WHERE action = 'report.export.requested'`,
	} {
		var got int
		require.NoError(t, service.db.QueryRow(ctx, query).Scan(&got))
		assert.Equal(t, want, got, "row count for %q", query)
	}
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
	setLotteryAllocationMode(t, service, ctx, event.EventID)

	extraEmployees := []Employee{
		{EmployeeID: "E3001", FullName: "Dora Wang", Department: "Engineering", Site: "Taipei HQ", JobGrade: 6, EmploymentStatus: "active"},
		{EmployeeID: "E3002", FullName: "Eric Chen", Department: "Sales", Site: "Taipei HQ", JobGrade: 4, EmploymentStatus: "active"},
	}
	for _, employee := range extraEmployees {
		_, err := service.db.Exec(ctx, `INSERT INTO employees (employee_id, full_name, department, site, job_grade, employment_status)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (employee_id) DO UPDATE SET full_name = EXCLUDED.full_name, department = EXCLUDED.department, site = EXCLUDED.site, job_grade = EXCLUDED.job_grade, employment_status = EXCLUDED.employment_status`,
			employee.EmployeeID, employee.FullName, employee.Department, employee.Site, employee.JobGrade, employee.EmploymentStatus)
		require.NoError(t, err)
	}

	registrations := []struct {
		employeeID     string
		idempotencyKey string
	}{
		{employeeID: "E1001", idempotencyKey: "lottery-received-1"},
		{employeeID: "E1002", idempotencyKey: "lottery-received-2"},
		{employeeID: "E3001", idempotencyKey: "lottery-received-3"},
		{employeeID: "E3002", idempotencyKey: "lottery-received-4"},
	}
	registrationIDs := make([]string, 0, len(registrations))
	for _, candidate := range registrations {
		booking, err := service.Book(ctx, Actor{ID: candidate.employeeID, Role: RoleEmployee}, event.EventID, BookingRequest{
			EmployeeID:     candidate.employeeID,
			IdempotencyKey: candidate.idempotencyKey,
		})
		require.NoError(t, err)
		require.Equal(t, RegistrationReceived, booking.Registration.Status)
		require.Nil(t, booking.Ticket)
		registrationIDs = append(registrationIDs, booking.Registration.RegistrationID)
	}

	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`, event.EventID, 0)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'received'`, event.EventID, 4)
	closeLotteryRegistration(t, service, ctx, event.EventID)

	run, err := service.RunLottery(ctx, admin, event.EventID, LotteryRunRequest{Seed: "det-seed-1"})
	require.NoError(t, err)
	assert.Equal(t, 3, run.WinnerCount)
	assert.Equal(t, 4, run.CandidateCount)
	assert.Equal(t, lotteryAlgorithmVersion, run.AlgorithmVersion)
	assert.False(t, run.InputSnapshotAt.IsZero())
	assert.NotEmpty(t, run.EligibilityRuleID)
	assert.Equal(t, 1, run.EligibilityRuleVersion)
	assert.Equal(t, RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"}, run.EligibilitySnapshot)

	var confirmedAfter int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`, event.EventID).Scan(&confirmedAfter))
	assert.Equal(t, 3, confirmedAfter)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'received'`, event.EventID, 0)

	seed := "det-seed-1"
	expectedWinners := lotteryExpectedWinners(seed, registrationIDs, 3)
	for _, registrationID := range registrationIDs {
		var status string
		require.NoError(t, service.db.QueryRow(ctx, `SELECT status FROM registrations WHERE registration_id = $1`, registrationID).Scan(&status))
		if expectedWinners[registrationID] {
			assert.Equal(t, RegistrationConfirmed, status, "registration %s", registrationID)
		} else {
			assert.Equal(t, RegistrationWaitlisted, status, "registration %s", registrationID)
		}
		var ticketCount int
		require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE registration_id = $1`, registrationID).Scan(&ticketCount))
		if expectedWinners[registrationID] {
			assert.Equal(t, 1, ticketCount, "winner %s ticket_count", registrationID)
		} else {
			assert.Equal(t, 0, ticketCount, "non-winner %s ticket_count", registrationID)
		}
	}

	var outboxCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'lottery.completed' AND aggregate_id = $1`, run.RunID).Scan(&outboxCount))
	assert.Equal(t, 1, outboxCount)
	var auditCount int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'lottery.completed' AND entity_type = 'event' AND entity_id = $1`, event.EventID).Scan(&auditCount))
	assert.Equal(t, 1, auditCount)
	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs WHERE action = 'lottery.completed' AND entity_id = $1`, event.EventID)
	assert.Equal(t, run.EligibilityRuleID, audit["eligibility_rule_id"])
	assert.Equal(t, float64(run.EligibilityRuleVersion), audit["eligibility_rule_version"])
	auditSnapshot, ok := audit["eligibility_snapshot"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "*", auditSnapshot["department"])
	assert.Equal(t, "*", auditSnapshot["site"])
	assert.Equal(t, float64(0), auditSnapshot["min_grade"])
	assert.Equal(t, "active", auditSnapshot["employment_status"])
	outbox := readOutboxPayloadMap(t, service, ctx, `SELECT payload::text FROM outbox_events WHERE event_type = 'lottery.completed' AND aggregate_id = $1`, run.RunID)
	assert.Equal(t, run.EligibilityRuleID, outbox["eligibility_rule_id"])
	assert.Equal(t, float64(run.EligibilityRuleVersion), outbox["eligibility_rule_version"])
	var winnerResults, waitlistResults int
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM lottery_results WHERE run_id = $1 AND result = 'winner'`, run.RunID).Scan(&winnerResults))
	require.NoError(t, service.db.QueryRow(ctx, `SELECT count(*) FROM lottery_results WHERE run_id = $1 AND result = 'waitlisted'`, run.RunID).Scan(&waitlistResults))
	assert.Equal(t, 3, winnerResults)
	assert.Equal(t, 1, waitlistResults)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'ticket.issued' AND entity_id IN (SELECT ticket_id FROM lottery_results WHERE run_id = $1 AND result = 'winner')`, run.RunID, 3)
}

func TestRunLotteryRecordsEligibilitySnapshotAfterRuleMutation(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Lottery Rule Snapshot Test",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	setLotteryAllocationMode(t, service, ctx, event.EventID)

	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "snapshot-book-engineering"})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E2001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E2001", IdempotencyKey: "snapshot-book-sales"})
	require.NoError(t, err)
	closeLotteryRegistration(t, service, ctx, event.EventID)

	salesRule := RuleInput{Department: "Sales", Site: "Taipei HQ", MinGrade: 4, EmploymentStatus: "active"}
	preview, err := service.UpdateEligibility(ctx, admin, event.EventID, UpdateEligibilityRequest{Rule: salesRule})
	require.NoError(t, err)
	assert.Equal(t, 1, preview.MatchCount)

	run, err := service.RunLottery(ctx, admin, event.EventID, LotteryRunRequest{Seed: "snapshot-seed"})
	require.NoError(t, err)
	assert.Equal(t, 2, run.CandidateCount)
	assert.Equal(t, 1, run.WinnerCount)
	assert.Equal(t, 2, run.EligibilityRuleVersion)
	assert.Equal(t, salesRule, run.EligibilitySnapshot)

	assertRowCount(t, service, ctx, `SELECT count(*) FROM lottery_results WHERE run_id = $1 AND employee_id = 'E2001'`, run.RunID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM lottery_results WHERE run_id = $1 AND employee_id = 'E1001' AND result = 'waitlisted'`, run.RunID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'received'`, event.EventID, 0)

	_, err = service.UpdateEligibility(ctx, admin, event.EventID, UpdateEligibilityRequest{
		Rule:           RuleInput{Department: "Legal", Site: "Remote", MinGrade: 9, EmploymentStatus: "active"},
		AllowZeroMatch: true,
	})
	require.NoError(t, err)
	existing, err := service.RunLottery(ctx, admin, event.EventID, LotteryRunRequest{Seed: "snapshot-seed"})
	require.NoError(t, err)
	assert.Equal(t, run.RunID, existing.RunID)
	assert.Equal(t, 2, existing.EligibilityRuleVersion)
	assert.Equal(t, salesRule, existing.EligibilitySnapshot)

	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs WHERE action = 'lottery.completed' AND entity_id = $1`, event.EventID)
	assert.Equal(t, float64(2), audit["eligibility_rule_version"])
	assert.Equal(t, float64(1), audit["ineligible_count"])
	auditSnapshot, ok := audit["eligibility_snapshot"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Sales", auditSnapshot["department"])
	assert.Equal(t, "Taipei HQ", auditSnapshot["site"])
	assert.Equal(t, float64(4), auditSnapshot["min_grade"])
	assert.Equal(t, "active", auditSnapshot["employment_status"])
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
	setLotteryAllocationMode(t, service, ctx, event.EventID)

	firstBooking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lottery-idem-book-1"})
	require.NoError(t, err)
	require.Equal(t, RegistrationReceived, firstBooking.Registration.Status)
	secondBooking, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "lottery-idem-book-2"})
	require.NoError(t, err)
	require.Equal(t, RegistrationReceived, secondBooking.Registration.Status)
	closeLotteryRegistration(t, service, ctx, event.EventID)

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

func TestRunLotteryReplaysSupersededRunByOriginalSeed(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Lottery Superseded Replay Test",
		Capacity: 1,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	setLotteryAllocationMode(t, service, ctx, event.EventID)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lottery-superseded-book"})
	require.NoError(t, err)
	closeLotteryRegistration(t, service, ctx, event.EventID)

	first, err := service.RunLottery(ctx, admin, event.EventID, LotteryRunRequest{Seed: "superseded-seed-a"})
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `UPDATE lottery_runs SET status = 'superseded' WHERE run_id = $1`, first.RunID)
	require.NoError(t, err)
	_, err = service.db.Exec(ctx, `INSERT INTO lottery_runs (run_id, event_id, seed, status, created_by, created_at)
		VALUES ('lot_superseded_replay_new', $1, 'superseded-seed-b', 'completed', $2, $3)`,
		event.EventID, admin.ID, service.now().Add(time.Minute))
	require.NoError(t, err)
	before := lotterySideEffectCounts(t, service, ctx, event.EventID)

	replayed, err := service.RunLottery(ctx, admin, event.EventID, LotteryRunRequest{Seed: "superseded-seed-a"})

	require.NoError(t, err)
	assert.Equal(t, first.RunID, replayed.RunID)
	assert.Equal(t, "superseded-seed-a", replayed.Seed)
	assert.Equal(t, "completed", replayed.Status)
	assert.Equal(t, before, lotterySideEffectCounts(t, service, ctx, event.EventID))
	var storedStatus string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT status FROM lottery_runs WHERE run_id = $1`, first.RunID).Scan(&storedStatus))
	assert.Equal(t, "superseded", storedStatus)
}

func TestRunLotteryRejectsDifferentSeedAfterCompletedRun(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Lottery Single Completed Test",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	setLotteryAllocationMode(t, service, ctx, event.EventID)
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lottery-single-1"})
	require.NoError(t, err)
	closeLotteryRegistration(t, service, ctx, event.EventID)

	first, err := service.RunLottery(ctx, admin, event.EventID, LotteryRunRequest{Seed: "single-seed-a"})
	require.NoError(t, err)
	_, err = service.RunLottery(ctx, admin, event.EventID, LotteryRunRequest{Seed: "single-seed-b"})
	require.Error(t, err)
	assert.Equal(t, 409, ErrorStatus(err))
	assert.Equal(t, "lottery run already completed for event", ErrorMessage(err))
	assertRowCount(t, service, ctx, `SELECT count(*) FROM lottery_runs WHERE event_id = $1`, event.EventID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'lottery.completed' AND aggregate_id = $1`, first.RunID, 1)
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

func setLotteryAllocationMode(t *testing.T, service *Service, ctx context.Context, eventID string) {
	t.Helper()
	_, err := service.db.Exec(ctx, `UPDATE events SET allocation_mode = $1 WHERE event_id = $2`, AllocationModeLottery, eventID)
	require.NoError(t, err)
}

func closeLotteryRegistration(t *testing.T, service *Service, ctx context.Context, eventID string) {
	t.Helper()
	_, err := service.db.Exec(ctx, `UPDATE events SET registration_close = $1 WHERE event_id = $2`, service.now().Add(-time.Minute), eventID)
	require.NoError(t, err)
}
