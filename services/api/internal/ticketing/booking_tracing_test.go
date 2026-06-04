package ticketing

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

func TestBookingStageSpanUsesBoundedStageAndOutcome(t *testing.T) {
	exporter, shutdown := installBookingTraceExporter(t)
	defer shutdown()

	ctx, finish := startBookingStageSpan(context.Background(), "event_lock")
	finish(RegistrationConfirmed)
	require.NotNil(t, ctx)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "booking.event_lock", spans[0].Name)
	assert.Contains(t, spans[0].Attributes, attribute.String("cets.booking.stage", "event_lock"))
	assert.Contains(t, spans[0].Attributes, attribute.String("cets.booking.outcome", RegistrationConfirmed))
}

func TestBookingStageSpanDoesNotAttachRawIdentifiers(t *testing.T) {
	exporter, shutdown := installBookingTraceExporter(t)
	defer shutdown()

	_, finish := startBookingStageSpan(context.Background(), "evt_secret_token")
	finish("idem_secret_key")

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "booking.unknown", spans[0].Name)
	for _, attr := range spans[0].Attributes {
		value := attr.Value.AsString()
		assert.NotContains(t, value, "evt_secret_token")
		assert.NotContains(t, value, "idem_secret_key")
	}
	assert.Contains(t, spans[0].Attributes, attribute.String("cets.booking.stage", "unknown"))
	assert.Contains(t, spans[0].Attributes, attribute.String("cets.booking.outcome", "unknown"))
}

func TestBookingStageSpanNestsUnderIncomingTraceContext(t *testing.T) {
	exporter, shutdown := installBookingTraceExporter(t)
	defer shutdown()

	parentCtx, parent := otel.Tracer("event-ticket-system/test").Start(context.Background(), "POST /api/v1/events/{event_id}/bookings")
	_, finish := startBookingStageSpan(parentCtx, "capacity")
	finish("success")
	parent.End()

	spans := exporter.GetSpans()
	require.Len(t, spans, 2)
	var parentID string
	var childParentID string
	for _, span := range spans {
		switch span.Name {
		case "POST /api/v1/events/{event_id}/bookings":
			parentID = span.SpanContext.SpanID().String()
		case "booking.capacity":
			childParentID = span.Parent.SpanID().String()
		}
	}
	require.NotEmpty(t, parentID)
	assert.Equal(t, parentID, childParentID)
}

func TestBookingReplaySpanUsesRegistrationOutcome(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	exporter, shutdown := installBookingTraceExporter(t)
	defer shutdown()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Replay Trace Outcome",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "trace-replay",
	})
	require.NoError(t, err)
	exporter.Reset()

	replay, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: "trace-replay",
	})
	require.NoError(t, err)
	require.Equal(t, RegistrationConfirmed, replay.Registration.Status)

	spans := exporter.GetSpans()
	var totalSpanFound bool
	for _, span := range spans {
		if span.Name != "booking.total" {
			continue
		}
		totalSpanFound = true
		assert.Contains(t, span.Attributes, attribute.String("cets.booking.outcome", RegistrationConfirmed))
		assert.NotEqual(t, "Error", span.Status.Code.String())
	}
	assert.True(t, totalSpanFound, "expected booking.total span on idempotency replay")
}

func installBookingTraceExporter(t *testing.T) (*tracetest.InMemoryExporter, func()) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	return exporter, func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	}
}
