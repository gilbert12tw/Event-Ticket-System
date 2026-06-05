package observability

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRedisOperationMetricsExposedInPrometheus(t *testing.T) {
	registry := NewRegistryWithIdentity("cets-backend", "backend-1")

	registry.ObserveRedisOperation("reserve", "ok", 12*time.Millisecond)
	registry.ObserveRedisOperation("reserve", "error", 150*time.Millisecond)
	registry.ObserveRedisOperation("commit", "ok", 5*time.Millisecond)

	var body bytes.Buffer
	registry.WritePrometheus(context.Background(), &body, nil)
	metrics := body.String()

	assert.Contains(t, metrics, `cets_redis_operation_total{operation="commit",result="ok"} 1`)
	assert.Contains(t, metrics, `cets_redis_operation_total{operation="reserve",result="ok"} 1`)
	assert.Contains(t, metrics, `cets_redis_operation_total{operation="reserve",result="error"} 1`)
	assert.Contains(t, metrics, `cets_redis_operation_seconds_bucket{operation="reserve",result="ok",le="0.025"} 1`)
	assert.Contains(t, metrics, `cets_redis_operation_seconds_count{operation="reserve",result="error"} 1`)
}

func TestRedisOperationMetricsBoundsUnknownLabels(t *testing.T) {
	registry := NewRegistry()

	registry.ObserveRedisOperation("evil-injection", "bad-result", time.Millisecond)

	var body bytes.Buffer
	registry.WritePrometheus(context.Background(), &body, nil)
	metrics := body.String()

	assert.Contains(t, metrics, `operation="unknown",result="unknown"`)
	assert.NotContains(t, metrics, "evil-injection")
	assert.NotContains(t, metrics, "bad-result")
}

func TestRedisOperationMetricsNilRegistrySafe(t *testing.T) {
	var r *Registry
	r.ObserveRedisOperation("reserve", "ok", time.Millisecond)
}
