package ticketing

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

type ReportObjectStore interface {
	Put(ctx context.Context, key string, contentType string, body []byte) error
}

type ReportObjectReader interface {
	Get(ctx context.Context, key string) ([]byte, string, error)
}

type ReportObjectExistenceChecker interface {
	Exists(ctx context.Context, key string) (bool, error)
}

func (s *Service) processReportExportOutbox(ctx context.Context, claim outboxClaim, store ReportObjectStore, retryPolicy OutboxRetryPolicy) (string, error) {
	if store == nil {
		return s.failReportExportOutbox(ctx, claim, retryPolicy, errors.New("report export object store is required"))
	}
	export, err := s.loadReportExport(ctx, claim.aggregateID)
	if err != nil {
		return s.failReportExportOutbox(ctx, claim, retryPolicy, err)
	}
	if export.Status == ReportExportStatusReady {
		return outboxAttemptOutcomePublished, s.markOutboxPublished(ctx, claim)
	}
	exists, err := reportExportObjectExists(ctx, store, export.ObjectKey)
	if err != nil {
		return s.failReportExportOutbox(ctx, claim, retryPolicy, err)
	}
	if exists {
		if err := s.markReportExportReady(ctx, export.ExportID); err != nil {
			return outboxAttemptOutcomeError, err
		}
		return outboxAttemptOutcomePublished, s.markOutboxPublished(ctx, claim)
	}
	body, err := s.buildReportExportCSV(ctx)
	if err != nil {
		return s.failReportExportOutbox(ctx, claim, retryPolicy, err)
	}
	if err := store.Put(ctx, export.ObjectKey, "text/csv; charset=utf-8", body); err != nil {
		return s.failReportExportOutbox(ctx, claim, retryPolicy, err)
	}
	if err := s.markReportExportReady(ctx, export.ExportID); err != nil {
		return outboxAttemptOutcomeError, err
	}
	return outboxAttemptOutcomePublished, s.markOutboxPublished(ctx, claim)
}

func reportExportObjectExists(ctx context.Context, store ReportObjectStore, key string) (bool, error) {
	checker, ok := store.(ReportObjectExistenceChecker)
	if !ok {
		return false, nil
	}
	return checker.Exists(ctx, key)
}

func (s *Service) loadReportExport(ctx context.Context, exportID string) (ReportExport, error) {
	var export ReportExport
	var completedAt sql.NullTime
	err := s.db.QueryRow(ctx, `SELECT export_id, requested_by, report_type, status, object_key, created_at,
			completed_at
		FROM report_exports WHERE export_id = $1`, exportID).
		Scan(&export.ExportID, &export.RequestedBy, &export.ReportType, &export.Status, &export.ObjectKey, &export.CreatedAt, &completedAt)
	if err != nil {
		return ReportExport{}, err
	}
	if completedAt.Valid {
		export.CompletedAt = &completedAt.Time
	}
	export.Format = ReportExportFormatCSV
	return export, nil
}

func (s *Service) buildReportExportCSV(ctx context.Context) ([]byte, error) {
	result, err := s.Reports(ctx, Actor{ID: "report-export-worker", Role: RoleHRAdmin})
	if err != nil {
		return nil, err
	}
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	if err := writer.Write([]string{"event_id", "title", "capacity_type", "capacity", "confirmed_count", "waitlist_count", "employee_count", "family_count", "total_attendee_count", "ticket_count", "checkin_count", "remaining_capacity", "city_distribution", "starts_at"}); err != nil {
		return nil, err
	}
	for _, row := range result.Rows {
		cityDistribution, err := cityDistributionCSVValue(row.CityDistribution)
		if err != nil {
			return nil, err
		}
		if err := writer.Write([]string{
			row.EventID,
			row.Title,
			row.CapacityType,
			nullableIntCSVValue(row.Capacity),
			strconv.Itoa(row.ConfirmedCount),
			strconv.Itoa(row.WaitlistCount),
			strconv.Itoa(row.EmployeeCount),
			strconv.Itoa(row.FamilyCount),
			strconv.Itoa(row.TotalAttendeeCount),
			strconv.Itoa(row.TicketCount),
			strconv.Itoa(row.CheckinCount),
			nullableIntCSVValue(row.RemainingCapacity),
			cityDistribution,
			row.StartsAt.UTC().Format(time.RFC3339),
		}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

func cityDistributionCSVValue(distribution map[string]int) (string, error) {
	if distribution == nil {
		distribution = map[string]int{}
	}
	payload, err := json.Marshal(distribution)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func nullableIntCSVValue(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func (s *Service) markReportExportReady(ctx context.Context, exportID string) error {
	_, err := s.db.Exec(ctx, `UPDATE report_exports SET status = $1, completed_at = now() WHERE export_id = $2`, ReportExportStatusReady, exportID)
	return err
}

func (s *Service) failReportExportOutbox(ctx context.Context, claim outboxClaim, retryPolicy OutboxRetryPolicy, failure error) (string, error) {
	outcome := outboxFailureOutcome(claim, retryPolicy)
	lastError := safeReportExportFailureError(failure)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return outboxAttemptOutcomeError, err
	}
	defer rollback(ctx, tx)
	if retryPolicy.exhausted(claim.attempts) {
		if _, err := tx.Exec(ctx, `UPDATE report_exports SET status = $1, completed_at = now() WHERE export_id = $2`, ReportExportStatusFailed, claim.aggregateID); err != nil {
			return outboxAttemptOutcomeError, err
		}
		if err := updateOutboxFailureInTx(ctx, tx, claim, retryPolicy, "dead_letter", lastError); err != nil {
			return outboxAttemptOutcomeError, err
		}
		if err := insertOutboxDeadLetterAuditInTx(ctx, tx, claim, outboxDeadLetterReasonRetryExhausted); err != nil {
			return outboxAttemptOutcomeError, err
		}
		return outcome, tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `UPDATE report_exports SET status = $1, completed_at = NULL WHERE export_id = $2`, ReportExportStatusPending, claim.aggregateID); err != nil {
		return outboxAttemptOutcomeError, err
	}
	if err := updateOutboxFailureInTx(ctx, tx, claim, retryPolicy, "pending", lastError); err != nil {
		return outboxAttemptOutcomeError, err
	}
	if err := insertOutboxRetryScheduledAuditInTx(ctx, tx, claim, outboxRetryReasonRetryableFailure); err != nil {
		return outboxAttemptOutcomeError, err
	}
	return outcome, tx.Commit(ctx)
}

func safeReportExportFailureError(failure error) string {
	if failure == nil {
		return ""
	}
	message := strings.TrimSpace(failure.Error())
	if message == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(message), "object store") {
		return "object store unavailable"
	}
	return "report export failed"
}
