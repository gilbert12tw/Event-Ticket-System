package observability

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestDependencySpanUsesBoundedServiceGraphAttributes(t *testing.T) {
	exporter, shutdown := installDependencyTraceExporter(t)
	defer shutdown()

	_, span := StartDependencySpan(context.Background(), DependencySpanConfig{
		System:      "redis",
		ServiceName: "redis",
		Operation:   "reserve",
	})
	EndDependencySpan(span, assert.AnError)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "redis.reserve", spans[0].Name)
	assert.Contains(t, spans[0].Attributes, attribute.String("peer.service", "redis"))
	assert.Contains(t, spans[0].Attributes, attribute.String("server.address", "redis"))
	assert.Contains(t, spans[0].Attributes, attribute.String("cets.dependency.system", "redis"))
	assert.Contains(t, spans[0].Attributes, attribute.String("cets.dependency.operation", "reserve"))
	assert.Equal(t, "Error", spans[0].Status.Code.String())
	for _, attr := range spans[0].Attributes {
		value := attr.Value.AsString()
		assert.NotContains(t, value, "token")
		assert.NotContains(t, value, "email@example.test")
		assert.NotContains(t, value, "secret")
	}
}

func installDependencyTraceExporter(t *testing.T) (*tracetest.InMemoryExporter, func()) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	return exporter, func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	}
}
