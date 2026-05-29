package ticketing

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

const outboxEventReportExportRequested = "report.export.requested"

type ReportObjectStore interface {
	Put(ctx context.Context, key string, contentType string, body []byte) error
}

func (s *Service) processReportExportOutbox(ctx context.Context, claim outboxClaim, store ReportObjectStore, maxAttempts int) error {
	if store == nil {
		return s.failReportExportOutbox(ctx, claim, maxAttempts, errors.New("report export object store is required"))
	}
	export, err := s.loadReportExport(ctx, claim.aggregateID)
	if err != nil {
		return s.failReportExportOutbox(ctx, claim, maxAttempts, err)
	}
	if export.Status == ReportExportStatusReady {
		return s.markOutboxPublished(ctx, claim.outboxID)
	}
	body, err := s.buildReportExportCSV(ctx)
	if err != nil {
		return s.failReportExportOutbox(ctx, claim, maxAttempts, err)
	}
	if err := store.Put(ctx, export.ObjectKey, "text/csv; charset=utf-8", body); err != nil {
		return s.failReportExportOutbox(ctx, claim, maxAttempts, err)
	}
	if err := s.markReportExportReady(ctx, export.ExportID); err != nil {
		return err
	}
	return s.markOutboxPublished(ctx, claim.outboxID)
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

func (s *Service) failReportExportOutbox(ctx context.Context, claim outboxClaim, maxAttempts int, failure error) error {
	if claim.attempts >= maxAttempts {
		if _, err := s.db.Exec(ctx, `UPDATE report_exports SET status = $1, completed_at = now() WHERE export_id = $2`, ReportExportStatusFailed, claim.aggregateID); err != nil {
			return err
		}
		return s.updateOutboxAfterSendFailure(ctx, claim.outboxID, claim.attempts, maxAttempts, failure.Error())
	}
	if _, err := s.db.Exec(ctx, `UPDATE report_exports SET status = $1, completed_at = NULL WHERE export_id = $2`, ReportExportStatusPending, claim.aggregateID); err != nil {
		return err
	}
	return s.updateOutboxAfterSendFailure(ctx, claim.outboxID, claim.attempts, maxAttempts, failure.Error())
}
