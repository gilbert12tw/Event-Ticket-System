package ticketing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"sort"
	"strings"
	"testing"
)

func TestCreateReportExportStartsPendingWithOutboxAndAudit(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	hr := Actor{ID: "hr-1", Role: RoleHRAdmin}
	got, err := service.CreateReportExport(ctx, hr, ReportExportRequest{ReportType: "participation"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ReportExportStatusPending {
		t.Fatalf("report export status = %q, want %q", got.Status, ReportExportStatusPending)
	}
	if got.ObjectKey == "" {
		t.Fatalf("report export object key must be set")
	}
	if !got.CompletedAt.IsZero() {
		t.Fatalf("pending report export should not be completed yet: %v", got.CompletedAt)
	}

	var status string
	var completedAt sql.NullTime
	if err := service.db.QueryRow(ctx, `SELECT status, completed_at FROM report_exports WHERE export_id = $1`, got.ExportID).Scan(&status, &completedAt); err != nil {
		t.Fatal(err)
	}
	if status != ReportExportStatusPending {
		t.Fatalf("stored report export status = %q, want %q", status, ReportExportStatusPending)
	}
	if completedAt.Valid {
		t.Fatalf("stored report export completed_at should be null while pending")
	}

	var outboxCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'report.export.requested' AND aggregate_id = $1`, got.ExportID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("report export outbox count = %d, want 1", outboxCount)
	}

	var auditCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'report.export.requested' AND entity_id = $1`, got.ExportID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("report export audit count = %d, want 1", auditCount)
	}
}

func TestGetReportExportRequiresHRAndReturnsPersistedStatus(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	hr := Actor{ID: "hr-1", Role: RoleHRAdmin}
	created, err := service.CreateReportExport(ctx, hr, ReportExportRequest{ReportType: "participation"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.GetReportExport(ctx, hr, created.ExportID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExportID != created.ExportID || got.Status != ReportExportStatusPending || got.ObjectKey == "" {
		t.Fatalf("export = %+v, want pending persisted export %s", got, created.ExportID)
	}
	if _, err := service.GetReportExport(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, created.ExportID); err == nil {
		t.Fatal("activity admin should not read report export status")
	}
}

func TestRunLotteryAllocatesDeterministicWinnersAndLeavesRemainingWaitlisted(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Lottery Deterministic Test",
		Capacity: 3,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lottery-book-1"}); err != nil {
		t.Fatal(err)
	}

	extraEmployees := []Employee{
		{EmployeeID: "E3001", FullName: "Dora Wang", Department: "Engineering", Site: "Taipei", JobGrade: 6, EmploymentStatus: "active"},
		{EmployeeID: "E3002", FullName: "Eric Chen", Department: "Sales", Site: "Taipei", JobGrade: 4, EmploymentStatus: "active"},
	}
	for _, employee := range extraEmployees {
		if _, err := service.db.Exec(ctx, `INSERT INTO employees (employee_id, full_name, department, site, job_grade, employment_status)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (employee_id) DO UPDATE SET full_name = EXCLUDED.full_name, department = EXCLUDED.department, site = EXCLUDED.site, job_grade = EXCLUDED.job_grade, employment_status = EXCLUDED.employment_status`,
			employee.EmployeeID, employee.FullName, employee.Department, employee.Site, employee.JobGrade, employee.EmploymentStatus); err != nil {
			t.Fatal(err)
		}
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
		if _, err := service.db.Exec(ctx, `INSERT INTO registrations (registration_id, event_id, employee_id, status, idempotency_key)
			VALUES ($1,$2,$3,'waitlisted',$4)`, candidate.registrationID, event.EventID, candidate.employeeID, candidate.idempotencyKey); err != nil {
			t.Fatal(err)
		}
	}

	var confirmedBefore int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`, event.EventID).Scan(&confirmedBefore); err != nil {
		t.Fatal(err)
	}
	if confirmedBefore != 1 {
		t.Fatalf("confirmed registrations before lottery = %d, want %d", confirmedBefore, 1)
	}

	run, err := service.RunLottery(ctx, admin, event.EventID, LotteryRunRequest{Seed: "det-seed-1"})
	if err != nil {
		t.Fatal(err)
	}
	if run.WinnerCount != 2 {
		t.Fatalf("run winner_count = %d, want %d", run.WinnerCount, 2)
	}

	var confirmedAfter int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`, event.EventID).Scan(&confirmedAfter); err != nil {
		t.Fatal(err)
	}
	if confirmedAfter != 3 {
		t.Fatalf("confirmed registrations after lottery = %d, want %d", confirmedAfter, 3)
	}

	seed := "det-seed-1"
	expectedWinners := lotteryExpectedWinners(seed, []string{
		"reg_wait_a", "reg_wait_b", "reg_wait_c",
	}, 2)
	for _, candidate := range waitlisted {
		var status string
		if err := service.db.QueryRow(ctx, `SELECT status FROM registrations WHERE registration_id = $1`, candidate.registrationID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if expectedWinners[candidate.registrationID] {
			if status != RegistrationConfirmed {
				t.Fatalf("registration %s status = %q, want %q", candidate.registrationID, status, RegistrationConfirmed)
			}
		} else if status != RegistrationWaitlisted {
			t.Fatalf("registration %s status = %q, want %q", candidate.registrationID, status, RegistrationWaitlisted)
		}
		var ticketCount int
		if err := service.db.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE registration_id = $1`, candidate.registrationID).Scan(&ticketCount); err != nil {
			t.Fatal(err)
		}
		if expectedWinners[candidate.registrationID] && ticketCount != 1 {
			t.Fatalf("winner %s ticket_count = %d, want %d", candidate.registrationID, ticketCount, 1)
		}
		if !expectedWinners[candidate.registrationID] && ticketCount != 0 {
			t.Fatalf("non-winner %s ticket_count = %d, want %d", candidate.registrationID, ticketCount, 0)
		}
	}

	var outboxCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'lottery.completed' AND aggregate_id = $1`, run.RunID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("lottery outbox count = %d, want %d", outboxCount, 1)
	}
	var auditCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'lottery.completed' AND entity_type = 'event' AND entity_id = $1`, event.EventID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("lottery audit count = %d, want %d", auditCount, 1)
	}
	var winnerResults, waitlistResults int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM lottery_results WHERE run_id = $1 AND result = 'winner'`, run.RunID).Scan(&winnerResults); err != nil {
		t.Fatal(err)
	}
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM lottery_results WHERE run_id = $1 AND result = 'waitlisted'`, run.RunID).Scan(&waitlistResults); err != nil {
		t.Fatal(err)
	}
	if winnerResults != 2 || waitlistResults != 1 {
		t.Fatalf("lottery results winners=%d waitlisted=%d, want 2/1", winnerResults, waitlistResults)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'ticket.issued' AND entity_id IN (SELECT ticket_id FROM lottery_results WHERE run_id = $1 AND result = 'winner')`, run.RunID, 2)
}

func TestRunLotteryReturnsExistingRunBySeed(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Lottery Idempotent Test",
		Capacity: 3,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "*", Site: "*", MinGrade: 0, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lottery-idem-book-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.Exec(ctx, `INSERT INTO registrations (registration_id, event_id, employee_id, status, idempotency_key)
		VALUES ('reg_wait_repeat_a', $1, 'E1002', 'waitlisted', 'lottery-idem-wait-1')`, event.EventID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.db.Exec(ctx, `INSERT INTO registrations (registration_id, event_id, employee_id, status, idempotency_key)
		VALUES ('reg_wait_repeat_b', $1, 'E2001', 'waitlisted', 'lottery-idem-wait-2')`, event.EventID); err != nil {
		t.Fatal(err)
	}

	first, err := service.RunLottery(ctx, admin, event.EventID, LotteryRunRequest{Seed: "repeat-seed"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.RunLottery(ctx, admin, event.EventID, LotteryRunRequest{Seed: "repeat-seed"})
	if err != nil {
		t.Fatal(err)
	}
	if second.RunID != first.RunID {
		t.Fatalf("second run_id = %s, want %s", second.RunID, first.RunID)
	}
	if second.WinnerCount != first.WinnerCount {
		t.Fatalf("second winner_count = %d, want %d", second.WinnerCount, first.WinnerCount)
	}

	var outboxCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'lottery.completed' AND aggregate_id = $1`, first.RunID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("lottery outbox count after repeated run = %d, want %d", outboxCount, 1)
	}
	var auditCount int
	if err := service.db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'lottery.completed' AND entity_type = 'event' AND entity_id = $1`, event.EventID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("lottery audit count after repeated run = %d, want %d", auditCount, 1)
	}
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
