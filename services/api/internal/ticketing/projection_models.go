package ticketing

// Inner event types carried inside a reporting.projection.update_required.v2 envelope.
const (
	projectionInnerTypeBookingConfirmed      = "booking.confirmed"
	projectionInnerTypeBookingCancelled      = "booking.cancelled"
	projectionInnerTypeBookingWaitlisted     = "booking.waitlisted"
	projectionInnerTypeBookingWaitlistCancel = "booking.waitlist_cancelled"
	projectionInnerTypeCheckinCompleted      = "checkin.completed"
)

// ProjectionEvent is the decoded inner payload extracted from a
// reporting.projection.update_required.v2 outbox envelope.
// It contains only the fields required for read-model aggregation.
// Never store employee IDs, names, or any PII here.
type ProjectionEvent struct {
	// EventID is the domain event (event_id in the events table).
	EventID string
	// OutboxID is the outbox row ID used as the idempotency offset.
	OutboxID string
	// TriggerEventID is the outbox_id of the original event that triggered this projection.
	TriggerEventID string
	// InnerType is the original domain event type (e.g. "booking.confirmed").
	InnerType string
	// Department label — only the label is stored, never employee identifiers.
	Department string
}

// eventSummaryRow holds the current aggregate counts read from
// reporting_event_summary before an upsert is computed.
type eventSummaryRow struct {
	ConfirmedCount      int
	CancelledCount      int
	WaitlistCount       int
	DepartmentBreakdown map[string]int
	LastEventOffset     string // TEXT outbox_id of the last applied event
}
