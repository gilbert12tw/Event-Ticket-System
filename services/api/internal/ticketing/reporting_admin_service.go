package ticketing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Service) CreateReportExport(ctx context.Context, actor Actor, req ReportExportRequest) (ReportExport, error) {
	if err := requireRole(actor, RoleHRAdmin); err != nil {
		return ReportExport{}, err
	}
	reportType, format, err := validateReportExportRequest(req)
	if err != nil {
		return ReportExport{}, err
	}
	exportID, err := newID("exp")
	if err != nil {
		return ReportExport{}, err
	}
	now := s.now()
	objectKey := "exports/" + exportID + ".csv"
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ReportExport{}, err
	}
	defer rollback(ctx, tx)
	_, err = tx.Exec(ctx, `INSERT INTO report_exports (export_id, requested_by, report_type, status, object_key, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`, exportID, actor.ID, reportType, ReportExportStatusPending, objectKey, now)
	if err != nil {
		return ReportExport{}, err
	}
	auditID, err := newID("aud")
	if err != nil {
		return ReportExport{}, err
	}
	if err := insertOutbox(ctx, tx, outboxEventReportExportRequestedV2, exportID, map[string]interface{}{
		"export_id":                exportID,
		"report_type":              reportType,
		"requested_by_employee_id": actor.ID,
		"filters":                  map[string]interface{}{},
		"requested_at":             now.UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return ReportExport{}, err
	}
	if err := insertAudit(ctx, tx, auditID, actor, "report.export.requested", "report_export", exportID, map[string]interface{}{"report_type": reportType, "object_key": objectKey}); err != nil {
		return ReportExport{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReportExport{}, err
	}
	return ReportExport{ExportID: exportID, RequestedBy: actor.ID, ReportType: reportType, Format: format, Status: ReportExportStatusPending, ObjectKey: objectKey, CreatedAt: now}, nil
}

func validateReportExportRequest(req ReportExportRequest) (string, string, error) {
	reportType := strings.TrimSpace(req.ReportType)
	if reportType == "" {
		return "", "", badRequest("report_type is required")
	}
	if reportType != ReportExportTypeParticipation {
		return "", "", badRequest("report_type must be participation")
	}
	format := strings.TrimSpace(req.Format)
	if format == "" {
		format = ReportExportFormatCSV
	}
	if format != ReportExportFormatCSV {
		return "", "", badRequest("format must be csv")
	}
	return reportType, format, nil
}

func (s *Service) GetReportExport(ctx context.Context, actor Actor, exportID string) (ReportExport, error) {
	if err := requireRole(actor, RoleHRAdmin); err != nil {
		return ReportExport{}, err
	}
	exportID = strings.TrimSpace(exportID)
	if exportID == "" {
		return ReportExport{}, badRequest("export_id is required")
	}
	export, err := s.loadReportExport(ctx, exportID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReportExport{}, notFound("report export not found")
	}
	return export, err
}

func (s *Service) RunLottery(ctx context.Context, actor Actor, eventID string, req LotteryRunRequest) (LotteryRun, error) {
	if err := requireRole(actor, RoleActivityAdmin); err != nil {
		return LotteryRun{}, err
	}
	seed := strings.TrimSpace(req.Seed)
	if seed == "" {
		seed = eventID
	}
	var existing LotteryRun
	err := s.db.QueryRow(ctx, `SELECT run_id, event_id, seed, status, winner_count, created_by, created_at FROM lottery_runs WHERE event_id = $1 AND seed = $2`, eventID, seed).
		Scan(&existing.RunID, &existing.EventID, &existing.Seed, &existing.Status, &existing.WinnerCount, &existing.CreatedBy, &existing.CreatedAt)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return LotteryRun{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return LotteryRun{}, err
	}
	defer rollback(ctx, tx)

	event, rule, err := s.lockEventWithRule(ctx, tx, eventID)
	if err != nil {
		return LotteryRun{}, err
	}
	err = tx.QueryRow(ctx, `SELECT run_id, event_id, seed, status, winner_count, created_by, created_at
		FROM lottery_runs WHERE event_id = $1 AND seed = $2`, eventID, seed).
		Scan(&existing.RunID, &existing.EventID, &existing.Seed, &existing.Status, &existing.WinnerCount, &existing.CreatedBy, &existing.CreatedAt)
	if err == nil {
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return LotteryRun{}, err
	}
	if err := validateLotteryEvent(event); err != nil {
		return LotteryRun{}, err
	}

	confirmed, err := s.confirmedCountTx(ctx, tx, eventID)
	if err != nil {
		return LotteryRun{}, err
	}

	winnerCount := 0
	candidates, err := s.lotteryCandidatesTx(ctx, tx, eventID, rule, seed)
	if err != nil {
		return LotteryRun{}, err
	}

	eventCapacity, err := limitedCapacity(event)
	if err != nil {
		return LotteryRun{}, err
	}
	capacity := max(eventCapacity-confirmed, 0)
	winnerLimit := len(candidates)
	if capacity < winnerLimit {
		winnerLimit = capacity
	}
	runID, err := newID("lot")
	if err != nil {
		return LotteryRun{}, err
	}
	now := s.now()
	_, err = tx.Exec(ctx, `INSERT INTO lottery_runs (run_id, event_id, seed, status, winner_count, created_by, created_at)
		VALUES ($1,$2,$3,'completed',0,$4,$5)`, runID, eventID, seed, actor.ID, now)
	if err != nil {
		return LotteryRun{}, err
	}
	for i := 0; i < winnerLimit; i++ {
		candidate := candidates[i]
		_, err := tx.Exec(ctx, `UPDATE registrations SET status = 'confirmed' WHERE registration_id = $1`, candidate.registrationID)
		if err != nil {
			return LotteryRun{}, err
		}
		ticket, err := s.createTicketTx(ctx, tx, candidate.registration, candidate.employee)
		if err != nil {
			return LotteryRun{}, err
		}
		if err := insertTicketIssuedAuditTx(ctx, tx, actor, ticket); err != nil {
			return LotteryRun{}, err
		}
		if err := insertLotteryResultTx(ctx, tx, runID, eventID, candidate.registrationID, candidate.registration.EmployeeID, "winner", ticket.TicketID, i); err != nil {
			return LotteryRun{}, err
		}
		winnerCount++
	}
	for i := winnerLimit; i < len(candidates); i++ {
		candidate := candidates[i]
		if err := insertLotteryResultTx(ctx, tx, runID, eventID, candidate.registrationID, candidate.registration.EmployeeID, "waitlisted", "", i); err != nil {
			return LotteryRun{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE lottery_runs SET winner_count = $2 WHERE run_id = $1`, runID, winnerCount); err != nil {
		return LotteryRun{}, err
	}
	auditID, err := newID("aud")
	if err != nil {
		return LotteryRun{}, err
	}
	if err := insertAudit(ctx, tx, auditID, actor, "lottery.completed", "event", eventID,
		map[string]interface{}{"seed": seed, "winner_count": winnerCount, "event_id": eventID}); err != nil {
		return LotteryRun{}, err
	}
	if err := insertOutbox(ctx, tx, "lottery.completed", runID,
		map[string]interface{}{"event_id": eventID, "seed": seed, "winner_count": winnerCount}); err != nil {
		return LotteryRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LotteryRun{}, err
	}
	return LotteryRun{RunID: runID, EventID: eventID, Seed: seed, Status: "completed", WinnerCount: winnerCount, CreatedBy: actor.ID, CreatedAt: now}, nil
}

func validateLotteryEvent(event Event) error {
	if event.Status != EventStatusPublished {
		return conflict("lottery can only run for published events")
	}
	if event.AllocationMode != AllocationModeLottery {
		return conflict("lottery allocation is not enabled for this event")
	}
	return nil
}

func insertLotteryResultTx(ctx context.Context, tx pgx.Tx, runID string, eventID string, registrationID string, employeeID string, result string, ticketID string, drawOrder int) error {
	resultID, err := newID("lor")
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO lottery_results
		(result_id, run_id, event_id, registration_id, employee_id, result, ticket_id, draw_order)
		VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7, ''),$8)`,
		resultID, runID, eventID, registrationID, employeeID, result, ticketID, drawOrder)
	return err
}

type lotteryCandidate struct {
	registrationID string
	registration   Registration
	employee       Employee
	drawSeed       string
}

func lotteryDrawKey(seed string, registrationID string) string {
	hash := sha256.Sum256([]byte(seed + "|" + registrationID))
	return hex.EncodeToString(hash[:])
}

func (s *Service) lotteryCandidatesTx(ctx context.Context, tx pgx.Tx, eventID string, rule EligibilityRule, seed string) ([]lotteryCandidate, error) {
	rows, err := tx.Query(ctx, `SELECT
			r.registration_id, r.event_id, r.employee_id, r.status, COALESCE(r.idempotency_key, ''), COALESCE(r.cancel_idempotency_key, ''),
			COALESCE(r.cancelled_at, '0001-01-01 00:01:00+00'::timestamptz), r.cancel_reason, r.family_count, r.created_at,
			e.full_name, e.department, e.site, e.job_grade, e.employment_status
		FROM registrations r
		JOIN employees e ON e.employee_id = r.employee_id
		WHERE r.event_id = $1 AND r.status = 'waitlisted'
		FOR UPDATE OF r`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []lotteryCandidate
	for rows.Next() {
		var reg Registration
		var employee Employee
		if err := rows.Scan(
			&reg.RegistrationID, &reg.EventID, &reg.EmployeeID, &reg.Status, &reg.IdempotencyKey,
			&reg.CancelKey, &reg.CancelledAt, &reg.CancelReason, &reg.FamilyCount, &reg.CreatedAt,
			&employee.FullName, &employee.Department, &employee.Site, &employee.JobGrade, &employee.EmploymentStatus,
		); err != nil {
			return nil, err
		}
		employee.EmployeeID = reg.EmployeeID
		if eligible, _ := EvaluateEligibility(employee, rule); !eligible {
			continue
		}
		candidates = append(candidates, lotteryCandidate{
			registrationID: reg.RegistrationID,
			registration:   reg,
			employee:       employee,
			drawSeed:       lotteryDrawKey(seed, reg.RegistrationID),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
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
	return candidates, nil
}
