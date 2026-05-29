package ticketing

import "time"

type OutboxQueueStatus struct {
	Queues []OutboxQueueStatusRow `json:"queues"`
}

type OutboxQueueStatusRow struct {
	Name            string     `json:"name"`
	Pending         int        `json:"pending"`
	InFlight        int        `json:"in_flight"`
	DeadLetter      int        `json:"dead_letter"`
	P95AgeSeconds   int        `json:"p95_age_seconds"`
	LastProcessedAt *time.Time `json:"last_processed_at,omitempty"`
}
