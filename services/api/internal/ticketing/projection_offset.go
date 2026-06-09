package ticketing

import (
	"fmt"
	"time"
)

// projectionOffsetKey builds the reporting_event_summary.last_event_offset value
// shared by the projection worker and the PH2-43 read-model rebuild. It is a
// fixed-width, time-ordered composite so the upsert idempotency guard
// (excluded.last_event_offset > reporting_event_summary.last_event_offset) and the
// GREATEST(...) merge compare lexicographically in true event order.
//
// Why not time.RFC3339Nano: RFC3339Nano strips trailing zeros from the fractional
// seconds, so its width is variable and lexical order != chronological order. For
// example "…56.1Z" sorts AFTER "…56.12Z" (the 'Z' byte, 0x5A, outranks '2', 0x32),
// inverting 100ms vs 120ms — common at the microsecond precision of a Postgres
// created_at, which would let an older event overwrite a newer aggregate.
//
// Zero-padded UnixNano (20 digits, wide enough for the int64 max) guarantees
// lexical order == chronological order. outboxID is appended as a deterministic
// tiebreaker for events sharing an instant. outbox_id itself is a random
// out_<hex> value (see ids.go) and is NOT time-sortable, so it can only serve as
// the tiebreaker, never the primary key.
func projectionOffsetKey(createdAt time.Time, outboxID string) string {
	return fmt.Sprintf("%020d|%s", createdAt.UTC().UnixNano(), outboxID)
}
