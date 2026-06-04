package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestQueryTracerUsesBoundedOperationSpan(t *testing.T) {
	exporter, shutdown := installPostgresTraceExporter(t)
	defer shutdown()
	tracer := queryTracer{
		serverAddress: "postgres",
		serverPort:    5432,
		database:      "cets",
	}

	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{
		SQL:  `/* booking_capacity */ SELECT count(*) FROM registrations WHERE event_id = $1`,
		Args: []any{"evt_secret"},
	})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{CommandTag: pgconn.NewCommandTag("SELECT 1")})

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "db.select", spans[0].Name)
	assert.Contains(t, spans[0].Attributes, attribute.String("db.system", "postgresql"))
	assert.Contains(t, spans[0].Attributes, attribute.String("db.operation", "select"))
	assert.Contains(t, spans[0].Attributes, attribute.String("cets.db.statement_class", "select"))
	assert.Contains(t, spans[0].Attributes, attribute.String("cets.db.command_tag", "select"))
	assert.Contains(t, spans[0].Attributes, attribute.String("server.address", "postgres"))
	assert.Contains(t, spans[0].Attributes, attribute.Int("server.port", 5432))
	assert.Contains(t, spans[0].Attributes, attribute.String("db.namespace", "cets"))
	for _, attr := range spans[0].Attributes {
		value := attr.Value.AsString()
		assert.NotContains(t, value, "registrations")
		assert.NotContains(t, value, "evt_secret")
	}
}

func TestQueryTracerMarksErrorsWithoutSQLOrArgs(t *testing.T) {
	exporter, shutdown := installPostgresTraceExporter(t)
	defer shutdown()
	tracer := queryTracer{serverAddress: "postgres", serverPort: 5432, database: "cets"}

	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{
		SQL:  `INSERT INTO employees (employee_id) VALUES ($1)`,
		Args: []any{"E1001"},
	})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{
		CommandTag: pgconn.NewCommandTag("INSERT 0 1"),
		Err:        assert.AnError,
	})

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "db.insert", spans[0].Name)
	assert.Equal(t, "Error", spans[0].Status.Code.String())
	for _, attr := range spans[0].Attributes {
		value := attr.Value.AsString()
		assert.NotContains(t, value, "employees")
		assert.NotContains(t, value, "E1001")
	}
}

func TestSQLOperationBoundsUnknownStatements(t *testing.T) {
	assert.Equal(t, "select", sqlOperation("  SELECT 1"))
	assert.Equal(t, "with", sqlOperation("WITH rows AS (SELECT 1) SELECT * FROM rows"))
	assert.Equal(t, "other", sqlOperation("VACUUM ANALYZE"))
	assert.Equal(t, "other", sqlOperation("/* unterminated"))
}

func TestQueryTracerOmitsEmptyDependencyAttributes(t *testing.T) {
	exporter, shutdown := installPostgresTraceExporter(t)
	defer shutdown()
	tracer := queryTracer{}

	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: `SELECT 1`})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{CommandTag: pgconn.NewCommandTag("SELECT 1")})

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.NotContains(t, spans[0].Attributes, attribute.String("server.address", ""))
	assert.NotContains(t, spans[0].Attributes, attribute.Int("server.port", 0))
	assert.NotContains(t, spans[0].Attributes, attribute.String("db.namespace", ""))
}

func TestConfigureQueryTracingAttachesPGXTracer(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://user:pass@localhost:5432/cets")
	require.NoError(t, err)

	configureQueryTracing(cfg)

	tracer, ok := cfg.ConnConfig.Tracer.(queryTracer)
	require.True(t, ok)
	assert.Equal(t, "localhost", tracer.serverAddress)
	assert.Equal(t, 5432, tracer.serverPort)
	assert.Equal(t, "cets", tracer.database)
}

func installPostgresTraceExporter(t *testing.T) (*tracetest.InMemoryExporter, func()) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	return exporter, func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(noop.NewTracerProvider())
	}
}
