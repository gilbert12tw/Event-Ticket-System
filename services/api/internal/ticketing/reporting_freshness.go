package ticketing

import "time"

// ReportMeta carries freshness metadata for a read-model-backed report response.
// It is included in every reports API response alongside the report data (AC-1).
type ReportMeta struct {
	Source              string     `json:"source"`
	GeneratedAt         *time.Time `json:"generated_at"`
	ReadModelLagSeconds int        `json:"read_model_lag_seconds"`
	IsStale             bool       `json:"is_stale"`
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
			Source:              "unavailable",
			GeneratedAt:         nil,
			ReadModelLagSeconds: -1,
			IsStale:             true,
		}
	}
	lag := int(time.Since(*updatedAt).Seconds())
	return ReportMeta{
		Source:              "read_model",
		GeneratedAt:         updatedAt,
		ReadModelLagSeconds: lag,
		IsStale:             lag >= thresholdSeconds,
	}
}
