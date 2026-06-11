package ticketing

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// PH2-45: CSV exports read the reporting projection, not the operational
// aggregate scans. Static event metadata (title, capacity, city, starts_at)
// still comes from a cheap PK join on events; every count comes exclusively
// from reporting_event_summary.

const (
	ReportExportSourceProjection  = "projection"
	ReportExportSourceOperational = "operational"

	// ReportExportStalePolicyFail is the only stale policy: a stale or
	// unavailable projection fails the export through the normal outbox
	// retry/dead-letter path (fail closed, never silently stale data).
	ReportExportStalePolicyFail = "fail"

	defaultExportStaleThresholdSeconds = 60

	reportProjectionStatusFresh   = "fresh"
	reportProjectionStatusPending = "pending_projection"
)

var (
	errReportingProjectionStale       = errors.New("reporting projection is stale")
	errReportingProjectionUnavailable = errors.New("reporting projection is unavailable")
)

// ReportExportSettings selects the export data source and the fail-closed
// staleness gate for the projection source. The zero value normalizes to the
// defaults (projection source, 60s threshold, fail policy).
type ReportExportSettings struct {
	Source                string
	StaleThresholdSeconds int
	StalePolicy           string
}

func (s ReportExportSettings) normalized() ReportExportSettings {
	if s.Source == "" {
		s.Source = ReportExportSourceProjection
	}
	if s.StaleThresholdSeconds <= 0 {
		s.StaleThresholdSeconds = defaultExportStaleThresholdSeconds
	}
	if s.StalePolicy == "" {
		s.StalePolicy = ReportExportStalePolicyFail
	}
	return s
}

// reportExportProjectionRow is one export row read from the projection.
// Pending rows (no projection row for the event yet) carry zero counts that
// the CSV writer renders as empty cells — never fabricated zeros (WS5-AC-4).
type reportExportProjectionRow struct {
	ReportRow
	ProjectionStatus string
}

// requireFreshProjectionForExport is the fail-closed staleness gate.
//
// Unavailable: the event_summary offsets row is missing (dropped tables,
// pre-first-rebuild deploy). Stale: unprocessed projection outbox backlog
// older than the threshold — the projection is demonstrably behind OLTP.
// Backlog age is used instead of offsets.updated_at age because updated_at
// only advances when a projection event is processed; in quiet periods (or
// while no publisher emits projection events) an updated_at-age gate would
// fail every export shortly after the last write despite the projection
// being correct.
//
// The gate is deliberately global: any stale backlog fails all exports, even
// for events whose own rows are current — conservative fail-closed over
// per-event precision.
func (s *Service) requireFreshProjectionForExport(ctx context.Context, thresholdSeconds int) error {
	var offsetRowExists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM reporting_projection_offsets WHERE projection_name = $1
		)`, projectionProjectionName).Scan(&offsetRowExists)
	if err != nil {
		return err
	}
	if !offsetRowExists {
		return errReportingProjectionUnavailable
	}

	var backlogStale bool
	err = s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM outbox_events
			WHERE event_type = $1
			  AND publish_status IN ('pending', 'processing')
			  AND created_at < now() - make_interval(secs => $2)
		)`, outboxEventReportingProjectionUpdateRequiredV2, thresholdSeconds).Scan(&backlogStale)
	if err != nil {
		return err
	}
	if backlogStale {
		return errReportingProjectionStale
	}
	return nil
}

// reportsFromProjection enumerates every event (PK metadata join) and takes
// all counts from reporting_event_summary. Events without a projection row
// are marked pending_projection.
func (s *Service) reportsFromProjection(ctx context.Context) ([]reportExportProjectionRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT e.event_id, e.title, e.capacity_type, e.capacity, e.event_city, e.starts_at,
		       s.event_id IS NOT NULL AS has_projection_row,
		       COALESCE(s.confirmed_count, 0), COALESCE(s.waitlist_count, 0),
		       COALESCE(s.employee_count, 0), COALESCE(s.family_count, 0),
		       COALESCE(s.ticket_count, 0), COALESCE(s.checkin_count, 0)
		FROM events e
		LEFT JOIN reporting_event_summary s ON s.event_id = e.event_id
		ORDER BY e.starts_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []reportExportProjectionRow
	for rows.Next() {
		var row reportExportProjectionRow
		var capacity pgtype.Int4
		var eventCity string
		var hasProjectionRow bool
		if err := rows.Scan(
			&row.EventID, &row.Title, &row.CapacityType, &capacity, &eventCity, &row.StartsAt,
			&hasProjectionRow,
			&row.ConfirmedCount, &row.WaitlistCount, &row.EmployeeCount, &row.FamilyCount,
			&row.TicketCount, &row.CheckinCount,
		); err != nil {
			return nil, err
		}
		row.ProjectionStatus = reportProjectionStatusFresh
		if !hasProjectionRow {
			row.ProjectionStatus = reportProjectionStatusPending
		}
		row.TotalAttendeeCount = row.EmployeeCount + row.FamilyCount
		row.CityDistribution = map[string]int{reportCityKey(eventCity): row.TotalAttendeeCount}
		if capacity.Valid {
			value := int(capacity.Int32)
			row.Capacity = &value
			row.RemainingCapacity = intPtr(max(value-row.ConfirmedCount, 0))
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// buildReportExportCSVFromProjection writes the Phase 1 whitelist columns in
// their original order plus a trailing projection_status column. Rows marked
// pending_projection get empty count cells — partial counts are never served
// as final (WS5-E-3); event metadata cells stay populated.
func (s *Service) buildReportExportCSVFromProjection(ctx context.Context, thresholdSeconds int) ([]byte, error) {
	if err := s.requireFreshProjectionForExport(ctx, thresholdSeconds); err != nil {
		return nil, err
	}
	rows, err := s.reportsFromProjection(ctx)
	if err != nil {
		return nil, err
	}
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	if err := writer.Write([]string{
		"event_id", "title", "capacity_type", "capacity", "confirmed_count", "waitlist_count",
		"employee_count", "family_count", "total_attendee_count", "ticket_count", "checkin_count",
		"remaining_capacity", "city_distribution", "starts_at", "projection_status",
	}); err != nil {
		return nil, err
	}
	for _, row := range rows {
		record, err := projectionExportCSVRecord(row)
		if err != nil {
			return nil, err
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

func projectionExportCSVRecord(row reportExportProjectionRow) ([]string, error) {
	if row.ProjectionStatus == reportProjectionStatusPending {
		return []string{
			row.EventID, row.Title, row.CapacityType, nullableIntCSVValue(row.Capacity),
			"", "", "", "", "", "", "", "", "",
			row.StartsAt.UTC().Format(time.RFC3339),
			row.ProjectionStatus,
		}, nil
	}
	cityDistribution, err := cityDistributionCSVValue(row.CityDistribution)
	if err != nil {
		return nil, err
	}
	return []string{
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
		row.ProjectionStatus,
	}, nil
}
