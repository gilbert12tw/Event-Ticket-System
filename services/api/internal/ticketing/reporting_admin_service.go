package ticketing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const lotteryAlgorithmVersion = "deterministic-sha256-v1"

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
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor, "report.export.requested", "report_export", exportID, map[string]interface{}{"report_type": reportType, "object_key": objectKey})); err != nil {
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
	existing, found, err := s.findLotteryRun(ctx, eventID, seed)
	if err != nil {
		return LotteryRun{}, err
	}
	if found {
		return existing, nil
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
	existing, found, err = findLotteryRunTx(ctx, tx, eventID, seed)
	if err != nil {
		return LotteryRun{}, err
	}
	if found {
		return existing, tx.Commit(ctx)
	}
	existing, found, err = findLatestLotteryRunForEventTx(ctx, tx, eventID)
	if err != nil {
		return LotteryRun{}, err
	}
	if found {
		if existing.Seed != seed {
			return LotteryRun{}, conflict("lottery run already completed for event")
		}
		return existing, tx.Commit(ctx)
	}
	if err := validateLotteryEvent(event, s.now()); err != nil {
		return LotteryRun{}, err
	}

	run, err := s.createLotteryRunTx(ctx, tx, actor, event, rule, eventID, seed)
	if err != nil {
		return LotteryRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LotteryRun{}, err
	}
	return run, nil
}

func (s *Service) findLotteryRun(ctx context.Context, eventID string, seed string) (LotteryRun, bool, error) {
	return scanLotteryRunRow(s.db.QueryRow(ctx, `SELECT run_id, event_id, seed, status, input_snapshot_at, algorithm_version, candidate_count,
			eligibility_rule_id, eligibility_rule_version, eligibility_snapshot, winner_count, created_by, created_at
		FROM lottery_runs WHERE event_id = $1 AND seed = $2 AND status IN ('completed', 'superseded')`, eventID, seed))
}

func findLotteryRunTx(ctx context.Context, tx pgx.Tx, eventID string, seed string) (LotteryRun, bool, error) {
	return scanLotteryRunRow(tx.QueryRow(ctx, `SELECT run_id, event_id, seed, status, input_snapshot_at, algorithm_version, candidate_count,
			eligibility_rule_id, eligibility_rule_version, eligibility_snapshot, winner_count, created_by, created_at
		FROM lottery_runs WHERE event_id = $1 AND seed = $2 AND status IN ('completed', 'superseded')`, eventID, seed))
}

func findLatestLotteryRunForEventTx(ctx context.Context, tx pgx.Tx, eventID string) (LotteryRun, bool, error) {
	return scanLotteryRunRow(tx.QueryRow(ctx, `SELECT run_id, event_id, seed, status, input_snapshot_at, algorithm_version, candidate_count,
			eligibility_rule_id, eligibility_rule_version, eligibility_snapshot, winner_count, created_by, created_at
		FROM lottery_runs WHERE event_id = $1 AND status = 'completed'
		ORDER BY created_at DESC
		LIMIT 1`, eventID))
}

func scanLotteryRunRow(row pgx.Row) (LotteryRun, bool, error) {
	var run LotteryRun
	var eligibilitySnapshot []byte
	err := row.Scan(&run.RunID, &run.EventID, &run.Seed, &run.Status, &run.InputSnapshotAt, &run.AlgorithmVersion, &run.CandidateCount,
		&run.EligibilityRuleID, &run.EligibilityRuleVersion, &eligibilitySnapshot, &run.WinnerCount, &run.CreatedBy, &run.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LotteryRun{}, false, nil
	}
	if err != nil {
		return LotteryRun{}, false, err
	}
	if len(eligibilitySnapshot) > 0 {
		if err := json.Unmarshal(eligibilitySnapshot, &run.EligibilitySnapshot); err != nil {
			return LotteryRun{}, false, err
		}
	}
	if run.Status == "superseded" {
		run.Status = "completed"
	}
	return run, true, nil
}

func (s *Service) createLotteryRunTx(ctx context.Context, tx pgx.Tx, actor Actor, event Event, rule EligibilityRule, eventID string, seed string) (LotteryRun, error) {
	candidates, err := s.lotteryCandidatesTx(ctx, tx, eventID, rule, seed)
	if err != nil {
		return LotteryRun{}, err
	}
	eventCapacity, err := limitedCapacity(event)
	if err != nil {
		return LotteryRun{}, err
	}
	confirmed, err := s.confirmedCountTx(ctx, tx, eventID)
	if err != nil {
		return LotteryRun{}, err
	}
	capacity := max(eventCapacity-confirmed, 0)
	eligibleCount := 0
	for _, candidate := range candidates {
		if candidate.eligible {
			eligibleCount++
		}
	}
	winnerLimit := eligibleCount
	if capacity < winnerLimit {
		winnerLimit = capacity
	}
	runID, err := newID("lot")
	if err != nil {
		return LotteryRun{}, err
	}
	now := s.now()
	eligibilitySnapshot := lotteryEligibilitySnapshot(rule)
	eligibilitySnapshotJSON, err := json.Marshal(eligibilitySnapshot)
	if err != nil {
		return LotteryRun{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO lottery_runs
			(run_id, event_id, seed, status, input_snapshot_at, algorithm_version, candidate_count,
			 eligibility_rule_id, eligibility_rule_version, eligibility_snapshot, winner_count, created_by, created_at)
		VALUES ($1,$2,$3,'completed',$4,$5,$6,$7,$8,$9::jsonb,0,$10,$11)`,
		runID, eventID, seed, now, lotteryAlgorithmVersion, len(candidates),
		rule.RuleID, rule.Version, string(eligibilitySnapshotJSON), actor.ID, now)
	if err != nil {
		return LotteryRun{}, err
	}
	winnerCount, err := s.insertLotteryDrawResultsTx(ctx, tx, actor, runID, eventID, candidates, winnerLimit)
	if err != nil {
		return LotteryRun{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE lottery_runs SET winner_count = $2 WHERE run_id = $1`, runID, winnerCount); err != nil {
		return LotteryRun{}, err
	}
	if err := insertLotteryCompletionEffectsTx(ctx, tx, lotteryCompletionEffects{
		actor:           actor,
		runID:           runID,
		eventID:         eventID,
		seed:            seed,
		rule:            rule,
		ruleSnapshot:    eligibilitySnapshot,
		winnerCount:     winnerCount,
		candidateCount:  len(candidates),
		ineligibleCount: len(candidates) - eligibleCount,
	}); err != nil {
		return LotteryRun{}, err
	}
	return LotteryRun{
		RunID:                  runID,
		EventID:                eventID,
		Seed:                   seed,
		Status:                 "completed",
		InputSnapshotAt:        now,
		AlgorithmVersion:       lotteryAlgorithmVersion,
		CandidateCount:         len(candidates),
		EligibilityRuleID:      rule.RuleID,
		EligibilityRuleVersion: rule.Version,
		EligibilitySnapshot:    eligibilitySnapshot,
		WinnerCount:            winnerCount,
		CreatedBy:              actor.ID,
		CreatedAt:              now,
	}, nil
}

func (s *Service) insertLotteryDrawResultsTx(ctx context.Context, tx pgx.Tx, actor Actor, runID string, eventID string, candidates []lotteryCandidate, winnerLimit int) (int, error) {
	winnerCount := 0
	for i := 0; i < winnerLimit; i++ {
		if err := s.insertLotteryWinnerTx(ctx, tx, actor, runID, eventID, candidates[i], i); err != nil {
			return 0, err
		}
		winnerCount++
	}
	for i := winnerLimit; i < len(candidates); i++ {
		if err := insertLotteryWaitlistResultTx(ctx, tx, runID, eventID, candidates[i], i); err != nil {
			return 0, err
		}
	}
	return winnerCount, nil
}

func (s *Service) insertLotteryWinnerTx(ctx context.Context, tx pgx.Tx, actor Actor, runID string, eventID string, candidate lotteryCandidate, drawOrder int) error {
	if _, err := tx.Exec(ctx, `UPDATE registrations SET status = 'confirmed' WHERE registration_id = $1`, candidate.registrationID); err != nil {
		return err
	}
	ticket, err := s.createTicketTx(ctx, tx, candidate.registration, candidate.employee)
	if err != nil {
		return err
	}
	if err := insertTicketIssuedAuditTx(ctx, tx, actor, ticket); err != nil {
		return err
	}
	return insertLotteryResultTx(ctx, tx, lotteryResultInsert{
		runID:          runID,
		eventID:        eventID,
		registrationID: candidate.registrationID,
		employeeID:     candidate.registration.EmployeeID,
		result:         "winner",
		ticketID:       ticket.TicketID,
		drawOrder:      drawOrder,
	})
}

func insertLotteryWaitlistResultTx(ctx context.Context, tx pgx.Tx, runID string, eventID string, candidate lotteryCandidate, drawOrder int) error {
	if _, err := tx.Exec(ctx, `UPDATE registrations SET status = 'waitlisted' WHERE registration_id = $1`, candidate.registrationID); err != nil {
		return err
	}
	return insertLotteryResultTx(ctx, tx, lotteryResultInsert{
		runID:          runID,
		eventID:        eventID,
		registrationID: candidate.registrationID,
		employeeID:     candidate.registration.EmployeeID,
		result:         "waitlisted",
		drawOrder:      drawOrder,
	})
}

type lotteryCompletionEffects struct {
	actor           Actor
	runID           string
	eventID         string
	seed            string
	rule            EligibilityRule
	ruleSnapshot    RuleInput
	winnerCount     int
	candidateCount  int
	ineligibleCount int
}

func insertLotteryCompletionEffectsTx(ctx context.Context, tx pgx.Tx, effects lotteryCompletionEffects) error {
	auditID, err := newID("aud")
	if err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"seed":                     effects.seed,
		"winner_count":             effects.winnerCount,
		"candidate_count":          effects.candidateCount,
		"ineligible_count":         effects.ineligibleCount,
		"algorithm_version":        lotteryAlgorithmVersion,
		"event_id":                 effects.eventID,
		"eligibility_rule_id":      effects.rule.RuleID,
		"eligibility_rule_version": effects.rule.Version,
		"eligibility_snapshot":     effects.ruleSnapshot,
	}
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, effects.actor, "lottery.completed", "event", effects.eventID,
		metadata)); err != nil {
		return err
	}
	return insertOutbox(ctx, tx, "lottery.completed", effects.runID,
		metadata)
}

func lotteryEligibilitySnapshot(rule EligibilityRule) RuleInput {
	return RuleInput{
		Department:       rule.Department,
		Site:             rule.Site,
		MinGrade:         rule.MinGrade,
		EmploymentStatus: rule.EmploymentStatus,
	}
}

func validateLotteryEvent(event Event, now time.Time) error {
	if event.Status != EventStatusPublished {
		return conflict("lottery can only run for published events")
	}
	if event.AllocationMode != AllocationModeLottery {
		return conflict("lottery allocation is not enabled for this event")
	}
	if !now.After(event.RegistrationClose) {
		return conflict("lottery can only run after registration window closes")
	}
	return nil
}

type lotteryResultInsert struct {
	runID          string
	eventID        string
	registrationID string
	employeeID     string
	result         string
	ticketID       string
	drawOrder      int
}

func insertLotteryResultTx(ctx context.Context, tx pgx.Tx, result lotteryResultInsert) error {
	resultID, err := newID("lor")
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO lottery_results
			(result_id, run_id, event_id, registration_id, employee_id, result, ticket_id, draw_order)
			VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7, ''),$8)`,
		resultID, result.runID, result.eventID, result.registrationID, result.employeeID, result.result, result.ticketID, result.drawOrder)
	return err
}

type lotteryCandidate struct {
	registrationID string
	registration   Registration
	employee       Employee
	drawSeed       string
	eligible       bool
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
		WHERE r.event_id = $1 AND r.status = 'received'
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
		eligible, _ := EvaluateEligibility(employee, rule)
		candidates = append(candidates, lotteryCandidate{
			registrationID: reg.RegistrationID,
			registration:   reg,
			employee:       employee,
			drawSeed:       lotteryDrawKey(seed, reg.RegistrationID),
			eligible:       eligible,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].eligible != candidates[j].eligible {
			return candidates[i].eligible
		}
		if !candidates[i].eligible {
			return candidates[i].registrationID < candidates[j].registrationID
		}
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
