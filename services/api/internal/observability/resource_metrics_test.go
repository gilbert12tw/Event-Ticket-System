package observability

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRuntimeResourceMetricsExposeCPUAndMemoryWithBoundedIdentity(t *testing.T) {
	registry := NewRegistryWithIdentity("cets-backend", "backend-1")

	var body bytes.Buffer
	registry.WritePrometheus(context.Background(), &body, nil)
	metrics := body.String()

	assert.Contains(t, metrics, "# TYPE process_cpu_seconds_total counter")
	assert.Contains(t, metrics, `process_cpu_seconds_total{service="cets-backend",replica="backend-1"}`)
	assert.Contains(t, metrics, "# TYPE go_memstats_heap_alloc_bytes gauge")
	assert.Contains(t, metrics, `go_memstats_heap_alloc_bytes{service="cets-backend",replica="backend-1"}`)
	assert.NotContains(t, metrics, "evt_secret")
}
