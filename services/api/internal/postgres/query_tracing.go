package postgres

import (
	"context"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const postgresTracerName = "event-ticket-system/postgres"

type queryTracer struct {
	serverAddress string
	serverPort    int
	database      string
}

func (t queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	operation := sqlOperation(data.SQL)
	ctx, span := otel.Tracer(postgresTracerName).Start(
		ctx,
		"db."+operation,
		trace.WithSpanKind(trace.SpanKindClient),
	)
	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", operation),
		attribute.String("cets.db.statement_class", operation),
	)
	for _, attr := range t.dependencyAttributes() {
		span.SetAttributes(attr)
	}
	return ctx
}

func (queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span := trace.SpanFromContext(ctx)
	if commandTag := data.CommandTag.String(); commandTag != "" {
		span.SetAttributes(attribute.String("cets.db.command_tag", boundedCommandTag(commandTag)))
	}
	if data.Err != nil {
		span.SetStatus(codes.Error, "postgres query failed")
	}
	span.End()
}

func (t queryTracer) dependencyAttributes() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 3)
	if t.serverAddress != "" {
		attrs = append(attrs, attribute.String("server.address", t.serverAddress))
	}
	if t.serverPort > 0 {
		attrs = append(attrs, attribute.Int("server.port", t.serverPort))
	}
	if t.database != "" {
		attrs = append(attrs, attribute.String("db.namespace", t.database))
	}
	return attrs
}

func sqlOperation(sql string) string {
	token := firstSQLToken(sql)
	switch token {
	case "select", "insert", "update", "delete", "begin", "commit", "rollback", "with", "create", "alter", "drop", "truncate":
		return token
	default:
		return "other"
	}
}

func firstSQLToken(sql string) string {
	sql = strings.TrimSpace(sql)
	for strings.HasPrefix(sql, "/*") {
		end := strings.Index(sql, "*/")
		if end < 0 {
			return ""
		}
		sql = strings.TrimSpace(sql[end+2:])
	}
	sql = strings.TrimLeftFunc(sql, func(r rune) bool {
		return unicode.IsSpace(r) || r == '('
	})
	var token strings.Builder
	for _, r := range sql {
		if !unicode.IsLetter(r) {
			break
		}
		token.WriteRune(unicode.ToLower(r))
	}
	return token.String()
}

func boundedCommandTag(commandTag string) string {
	operation := sqlOperation(commandTag)
	if operation != "other" {
		return operation
	}
	return "other"
}
