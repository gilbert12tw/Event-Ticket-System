package ticketing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const bookingStageTracerName = "event-ticket-system/ticketing/booking"

func startBookingStageSpan(ctx context.Context, stage string) (context.Context, func(string)) {
	stage = boundedBookingTraceLabel(stage)
	ctx, span := otel.Tracer(bookingStageTracerName).Start(
		ctx,
		"booking."+stage,
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	span.SetAttributes(attribute.String("cets.booking.stage", stage))
	return ctx, func(outcome string) {
		outcome = boundedBookingTraceLabel(outcome)
		span.SetAttributes(attribute.String("cets.booking.outcome", outcome))
		if outcome == "error" {
			span.SetStatus(codes.Error, "booking stage failed")
		}
		span.End()
	}
}

func boundedBookingTraceLabel(value string) string {
	switch value {
	case "total", "preadmission", "tx", "begin_tx", "idempotency_lock", "event_lock", "validate",
		"duplicate_lookup", "capacity", "create_response", "commit", "success", "error",
		RegistrationConfirmed, RegistrationWaitlisted, RegistrationReceived:
		return value
	default:
		return "unknown"
	}
}
