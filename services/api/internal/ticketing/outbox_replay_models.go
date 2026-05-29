package ticketing

import "time"

type ReplayOutboxRequest struct {
	Kind       string
	From       time.Time
	To         time.Time
	EventTypes []string
	DryRun     bool
}

type ReplayOutboxResult struct {
	Kind          string    `json:"kind"`
	From          time.Time `json:"from"`
	To            time.Time `json:"to"`
	DryRun        bool      `json:"dry_run"`
	AffectedCount int       `json:"affected_count"`
	EnqueuedCount *int      `json:"enqueued_count,omitempty"`
	AuditID       string    `json:"audit_id,omitempty"`
}
