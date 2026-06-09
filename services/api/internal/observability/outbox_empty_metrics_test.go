package observability

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOutboxMetricsExposeEmptyQueueBaseline(t *testing.T) {
	db := fakeSQLMetricsDB{lockWaitCount: 0}

	var body bytes.Buffer
	NewRegistry().WritePrometheus(context.Background(), &body, db)
	metrics := body.String()

	assert.Contains(t, metrics, `cets_outbox_pending_total{event_type="none",worker_kind="none",status="empty"} 0`)
	assert.Contains(t, metrics, `cets_outbox_oldest_lag_seconds{event_type="none",worker_kind="none",status="empty"} 0`)
	assert.Contains(t, metrics, `cets_outbox_retry_count{event_type="none",worker_kind="none",status="empty"} 0`)
	assert.Contains(t, metrics, `cets_outbox_lease_held_seconds{event_type="none",worker_kind="none",status="empty"} 0`)
	assert.Contains(t, metrics, `cets_outbox_dead_letter_total{event_type="none",worker_kind="none"} 0`)
}
