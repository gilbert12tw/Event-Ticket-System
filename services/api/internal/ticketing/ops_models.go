package ticketing

import "time"

type CapacityPressure struct {
	Events []CapacityPressureRow `json:"events"`
}

type CapacityPressureRow struct {
	EventID                 string `json:"event_id"`
	CapacityType            string `json:"capacity_type"`
	ConfirmedCount          int    `json:"confirmed_count"`
	WaitlistCount           int    `json:"waitlist_count"`
	ReceivedCount           int    `json:"received_count"`
	RemainingCapacity       *int   `json:"remaining_capacity"`
	ReservationCount        int    `json:"reservation_count"`
	ReservationState        string `json:"reservation_state"`
	RejectedPerMin          *int   `json:"rejected_per_min"`
	RateLimitDropPerMin     *int   `json:"rate_limit_drop_per_min"`
	IdempotencyReplayPerMin *int   `json:"idempotency_replay_per_min"`
}

type ReportFreshness struct {
	Projections []ReportFreshnessProjection `json:"projections"`
}

type ReportFreshnessProjection struct {
	Name              string     `json:"name"`
	LastApplied       *time.Time `json:"last_applied"`
	LagSeconds        int        `json:"lag_seconds"`
	Degraded          bool       `json:"degraded"`
	RebuildInProgress bool       `json:"rebuild_in_progress,omitempty"`
	RebuildStartedAt  *time.Time `json:"rebuild_started_at,omitempty"`
}

type OpsDashboard struct {
	CapacityPressure CapacityPressure             `json:"capacity_pressure"`
	Queues           OutboxQueueStatus            `json:"queues"`
	ReportsFreshness ReportFreshness              `json:"reports_freshness"`
	DeadLetterRecent []NotificationDeliveryOpsRow `json:"dead_letter_recent,omitempty"`
	ReplayRecent     []OutboxReplayRecentRow      `json:"replay_recent,omitempty"`
}

type OutboxReplayRecentRow struct {
	AuditID       string    `json:"audit_id"`
	ActorRole     string    `json:"actor_role"`
	Kind          string    `json:"kind"`
	DryRun        bool      `json:"dry_run"`
	AffectedCount int       `json:"affected_count"`
	EnqueuedCount *int      `json:"enqueued_count,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}
