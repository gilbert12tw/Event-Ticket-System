package ticketing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const workerTracerName = "event-ticket-system/ticketing/worker"

func restoreTraceContext(ctx context.Context, traceparent string) context.Context {
	if traceparent == "" {
		return ctx
	}
	carrier := propagation.MapCarrier{"traceparent": traceparent}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

func startWorkerSpan(ctx context.Context, claim outboxClaim) (context.Context, trace.Span) {
	ctx = restoreTraceContext(ctx, claim.traceContext)
	ctx, span := otel.Tracer(workerTracerName).Start(
		ctx,
		"worker.process "+safeOutboxTelemetryEventType(claim.eventType),
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	span.SetAttributes(
		attribute.String("cets.outbox_id", claim.outboxID),
		attribute.String("cets.event_type", safeOutboxTelemetryEventType(claim.eventType)),
		attribute.String("cets.worker_kind", safeOutboxTelemetryWorkerKind(claim.eventType)),
		attribute.Int("cets.attempt", claim.attempts),
	)
	return ctx, span
}

func endWorkerSpan(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, "worker processing failed")
		span.RecordError(err)
	}
	span.End()
}
