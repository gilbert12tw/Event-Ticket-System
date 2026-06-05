package observability

import (
	"context"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const dependencyTracerName = "event-ticket-system/dependency"

type DependencySpanConfig struct {
	System      string
	ServiceName string
	Operation   string
}

func StartDependencySpan(ctx context.Context, cfg DependencySpanConfig) (context.Context, trace.Span) {
	system := cleanDependencyValue(cfg.System, "unknown")
	serviceName := cleanDependencyValue(cfg.ServiceName, system)
	operation := cleanDependencyValue(cfg.Operation, "operation")
	ctx, span := otel.Tracer(dependencyTracerName).Start(
		ctx,
		system+"."+operation,
		trace.WithSpanKind(trace.SpanKindClient),
	)
	span.SetAttributes(
		attribute.String("peer.service", serviceName),
		attribute.String("server.address", serviceName),
		attribute.String("cets.dependency.system", system),
		attribute.String("cets.dependency.operation", operation),
	)
	return ctx, span
}

func EndDependencySpan(span trace.Span, err error) {
	if err != nil {
		span.SetStatus(codes.Error, "dependency call failed")
	}
	span.End()
}

func SetDependencyHTTPStatus(span trace.Span, statusCode int) {
	if statusCode <= 0 {
		return
	}
	span.SetAttributes(attribute.Int("http.response.status_code", statusCode))
	if statusCode >= http.StatusInternalServerError {
		span.SetStatus(codes.Error, http.StatusText(statusCode))
	}
}

func cleanDependencyValue(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
