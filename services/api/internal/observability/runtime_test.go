package observability

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestStartRuntimeNoopsWhenTelemetryDisabled(t *testing.T) {
	runtimeObs, err := StartRuntime(context.Background(), RuntimeConfig{}, nil)
	require.NoError(t, err)
	require.NoError(t, runtimeObs.Shutdown(context.Background()))
}

func TestStartRuntimeRequiresEnabledTelemetrySettings(t *testing.T) {
	_, err := StartRuntime(context.Background(), RuntimeConfig{OTelTracesEnabled: true}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OTEL_EXPORTER_OTLP_ENDPOINT")

	_, err = StartRuntime(context.Background(), RuntimeConfig{PyroscopeEnabled: true}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PYROSCOPE_SERVER_ADDRESS")
}

func TestStartRuntimeTracingUsesDefaultServiceName(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	runtimeObs, err := StartRuntime(context.Background(), RuntimeConfig{
		OTelTracesEnabled:        true,
		OTelExporterOTLPEndpoint: "http://127.0.0.1:4318",
	}, logger)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, runtimeObs.Shutdown(context.Background()))
		otel.SetTracerProvider(noop.NewTracerProvider())
	})

	assert.Contains(t, logs.String(), defaultServiceName)
}

func TestStartRuntimeProfilingUsesDefaultServiceName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	runtimeObs, err := StartRuntime(context.Background(), RuntimeConfig{
		PyroscopeEnabled:       true,
		PyroscopeServerAddress: server.URL,
		PyroscopeUploadRate:    time.Hour,
	}, logger)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, runtimeObs.Shutdown(context.Background()))
	})

	assert.Contains(t, logs.String(), defaultServiceName)
}

func TestTraceHTTPUsesRoutePatternWithoutRawPath(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	})

	handler := TraceHTTP(func(*http.Request) string {
		return "/api/v1/events/{event_id}"
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/evt_secret_token", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "POST /api/v1/events/{event_id}", spans[0].Name)
	assert.Contains(t, spans[0].Attributes, attribute.String("cets.route", "/api/v1/events/{event_id}"))
	assert.NotContains(t, spans[0].Name, "evt_secret_token")
	for _, attr := range spans[0].Attributes {
		assert.NotContains(t, attr.Value.AsString(), "evt_secret_token")
	}
}
