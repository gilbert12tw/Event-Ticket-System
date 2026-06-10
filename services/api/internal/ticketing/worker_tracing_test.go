package ticketing

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func installWorkerTraceExporter(t *testing.T) (*tracetest.InMemoryExporter, func()) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prev := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return exporter, func() {
		otel.SetTracerProvider(prev)
		otel.SetTextMapPropagator(prevProp)
		_ = tp.Shutdown(context.Background())
	}
}

func TestStartWorkerSpanCreatesConsumerSpan(t *testing.T) {
	exporter, shutdown := installWorkerTraceExporter(t)
	defer shutdown()

	claim := outboxClaim{
		outboxID:  "out_abc123",
		eventType: "booking.confirmed",
		attempts:  1,
	}
	ctx, span := startWorkerSpan(context.Background(), claim)
	span.End()

	require.NotNil(t, ctx)
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "worker.process booking.confirmed", spans[0].Name)
	assert.Equal(t, trace.SpanKindConsumer, spans[0].SpanKind)
	assert.Contains(t, spans[0].Attributes, attribute.String("cets.outbox_id", "out_abc123"))
	assert.Contains(t, spans[0].Attributes, attribute.String("cets.event_type", "booking.confirmed"))
	assert.Contains(t, spans[0].Attributes, attribute.Int("cets.attempt", 1))
}

func TestStartWorkerSpanLinksToParentTrace(t *testing.T) {
	exporter, shutdown := installWorkerTraceExporter(t)
	defer shutdown()

	parentCtx, parentSpan := otel.Tracer("test").Start(context.Background(), "http.request")
	parentTraceID := parentSpan.SpanContext().TraceID().String()
	parentSpanID := parentSpan.SpanContext().SpanID().String()
	parentSpan.End()

	traceparent := "00-" + parentTraceID + "-" + parentSpanID + "-01"
	claim := outboxClaim{
		outboxID:     "out_xyz",
		eventType:    "booking.confirmed.v2",
		attempts:     2,
		traceContext: traceparent,
	}
	_ = parentCtx
	_, span := startWorkerSpan(context.Background(), claim)
	span.End()

	spans := exporter.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2)
	workerSpan := spans[len(spans)-1]
	assert.Equal(t, parentTraceID, workerSpan.SpanContext.TraceID().String())
}

func TestRestoreTraceContextEmptyTraceparent(t *testing.T) {
	ctx := restoreTraceContext(context.Background(), "")
	sc := trace.SpanContextFromContext(ctx)
	assert.False(t, sc.IsValid())
}

func TestRestoreTraceContextValidTraceparent(t *testing.T) {
	_, shutdown := installWorkerTraceExporter(t)
	defer shutdown()

	traceparent := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	ctx := restoreTraceContext(context.Background(), traceparent)
	sc := trace.SpanContextFromContext(ctx)
	assert.True(t, sc.IsValid())
	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", sc.TraceID().String())
}

func TestEndWorkerSpanRecordsError(t *testing.T) {
	exporter, shutdown := installWorkerTraceExporter(t)
	defer shutdown()

	claim := outboxClaim{outboxID: "out_err", eventType: "booking.confirmed", attempts: 1}
	_, span := startWorkerSpan(context.Background(), claim)
	endWorkerSpan(span, errors.New("db connection lost"))

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "Error", spans[0].Status.Code.String())
}

func TestEndWorkerSpanNoErrorOK(t *testing.T) {
	exporter, shutdown := installWorkerTraceExporter(t)
	defer shutdown()

	claim := outboxClaim{outboxID: "out_ok", eventType: "booking.confirmed", attempts: 1}
	_, span := startWorkerSpan(context.Background(), claim)
	endWorkerSpan(span, nil)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "Unset", spans[0].Status.Code.String())
}

func TestExtractOutboxTraceContextValid(t *testing.T) {
	payload := `{"event_id":"out_1","event_type":"booking.confirmed","schema_version":2,"trace_context":{"traceparent":"00-abc123-def456-01"},"payload":{}}`
	tp := extractOutboxTraceContext(payload)
	assert.Equal(t, "00-abc123-def456-01", tp)
}

func TestExtractOutboxTraceContextMissing(t *testing.T) {
	payload := `{"event_id":"out_1","event_type":"booking.confirmed","schema_version":2,"payload":{}}`
	tp := extractOutboxTraceContext(payload)
	assert.Empty(t, tp)
}

func TestExtractOutboxTraceContextInvalidJSON(t *testing.T) {
	tp := extractOutboxTraceContext("not json")
	assert.Empty(t, tp)
}
