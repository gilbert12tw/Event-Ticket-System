package ticketing

import (
	"bytes"
	"context"
	"encoding/csv"
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
	err := s.db.QueryRow(ctx, `SELECT export_id, requested_by, report_type, status, object_key, created_at,
			COALESCE(completed_at, '0001-01-01 00:00:00+00'::timestamptz)
		FROM report_exports WHERE export_id = $1`, exportID).
		Scan(&export.ExportID, &export.RequestedBy, &export.ReportType, &export.Status, &export.ObjectKey, &export.CreatedAt, &export.CompletedAt)
	if err != nil {
		return ReportExport{}, err
	}
	return export, nil
}

func (s *Service) buildReportExportCSV(ctx context.Context) ([]byte, error) {
	rows, err := s.Reports(ctx, Actor{ID: "report-export-worker", Role: RoleHRAdmin})
	if err != nil {
		return nil, err
	}
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	if err := writer.Write([]string{"event_id", "title", "capacity", "confirmed_count", "waitlist_count", "ticket_count", "checkin_count", "remaining_capacity", "starts_at"}); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := writer.Write([]string{
			row.EventID,
			row.Title,
			strconv.Itoa(row.Capacity),
			strconv.Itoa(row.ConfirmedCount),
			strconv.Itoa(row.WaitlistCount),
			strconv.Itoa(row.TicketCount),
			strconv.Itoa(row.CheckinCount),
			strconv.Itoa(row.RemainingCapacity),
			row.StartsAt.UTC().Format(time.RFC3339),
		}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

func (s *Service) markReportExportReady(ctx context.Context, exportID string) error {
	_, err := s.db.Exec(ctx, `UPDATE report_exports SET status = $1, completed_at = now() WHERE export_id = $2`, ReportExportStatusReady, exportID)
	return err
}

func (s *Service) failReportExportOutbox(ctx context.Context, claim outboxClaim, maxAttempts int, failure error) error {
	if _, err := s.db.Exec(ctx, `UPDATE report_exports SET status = $1, completed_at = now() WHERE export_id = $2`, ReportExportStatusFailed, claim.aggregateID); err != nil {
		return err
	}
	return s.updateOutboxAfterSendFailure(ctx, claim.outboxID, claim.attempts, maxAttempts, failure)
}
