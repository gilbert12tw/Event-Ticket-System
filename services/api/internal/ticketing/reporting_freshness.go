package ticketing

import "time"

const (
	ReportSourceReportingProjection = "reporting_projection"
	ReportSourceUnavailable         = "unavailable"
)

// ReportMeta carries freshness metadata for a read-model-backed report response.
// It is included in every reports API response alongside the report data (AC-1).
type ReportMeta struct {
	AsOf       *time.Time `json:"as_of"`
	LagSeconds int        `json:"lag_seconds"`
	Degraded   bool       `json:"degraded"`
	Source     string     `json:"source"`
}

// ReportsResult wraps report rows with their projection freshness timestamp.
// The handler calls FreshnessFromProjection on ProjectionUpdatedAt to build ReportMeta.
type ReportsResult struct {
	Rows                []ReportRow
	ProjectionUpdatedAt *time.Time // nil when the offset row is absent from the DB
}

// FreshnessFromProjection builds a ReportMeta from a projection offset row.
// updatedAt is nil when the projection row does not exist (AC-4).
// This function is pure — it makes no DB calls and is unit-testable without a database.
func FreshnessFromProjection(updatedAt *time.Time, thresholdSeconds int) ReportMeta {
	if updatedAt == nil {
		return ReportMeta{
			AsOf:       nil,
			LagSeconds: -1,
			Degraded:   true,
			Source:     ReportSourceUnavailable,
		}
	}
	lag := int(time.Since(*updatedAt).Seconds())
	if lag < 0 {
		lag = 0
	}
	return ReportMeta{
		AsOf:       updatedAt,
		LagSeconds: lag,
		Degraded:   lag > thresholdSeconds,
		Source:     ReportSourceReportingProjection,
	}
}
