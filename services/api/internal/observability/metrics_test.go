package observability

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHTTPMetricsExposeREDSignalsWithBoundedLabels(t *testing.T) {
	registry := NewRegistry()

	registry.ObserveHTTPRequest("/api/v1/events/{event_id}", "GET", 201, 80*time.Millisecond)

	var body bytes.Buffer
	registry.WritePrometheus(context.Background(), &body, nil)
	metrics := body.String()

	assert.Contains(t, metrics, `cets_http_requests_total{route="/api/v1/events/{event_id}",method="GET",status_class="2xx"} 1`)
	assert.Contains(t, metrics, `cets_http_request_seconds_bucket{route="/api/v1/events/{event_id}",method="GET",status_class="2xx",le="0.1"} 1`)
	assert.Contains(t, metrics, `cets_http_request_seconds_count{route="/api/v1/events/{event_id}",method="GET",status_class="2xx"} 1`)
	assert.NotContains(t, metrics, "evt_secret")
}

func TestHTTPMetricsCollapseUnknownMethodsToBoundedLabel(t *testing.T) {
	registry := NewRegistry()

	registry.ObserveHTTPRequest("/unknown", "FOOBAR-SCANNER-TOKEN", 404, 5*time.Millisecond)

	var body bytes.Buffer
	registry.WritePrometheus(context.Background(), &body, nil)
	metrics := body.String()

	assert.Contains(t, metrics, `cets_http_requests_total{route="/unknown",method="UNKNOWN",status_class="4xx"} 1`)
	assert.NotContains(t, metrics, "FOOBAR-SCANNER-TOKEN")
}

func TestHTTPMetricsNormalizeKnownMethodCase(t *testing.T) {
	registry := NewRegistry()

	registry.ObserveHTTPRequest("/healthz", "get", 200, time.Millisecond)

	var body bytes.Buffer
	registry.WritePrometheus(context.Background(), &body, nil)

	assert.Contains(t, body.String(), `method="GET"`)
}

func TestHTTPMetricsEscapeLabels(t *testing.T) {
	registry := NewRegistry()

	registry.ObserveHTTPRequest(`/api/"quoted"`, "GET", 500, time.Second)

	var body bytes.Buffer
	registry.WritePrometheus(context.Background(), &body, nil)

	assert.Contains(t, body.String(), `route="/api/\"quoted\""`)
	assert.Contains(t, body.String(), `status_class="5xx"`)
}
