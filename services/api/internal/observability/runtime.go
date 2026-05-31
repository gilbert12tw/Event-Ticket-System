package observability

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/grafana/pyroscope-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

type RuntimeConfig struct {
	ServiceName                  string
	ServiceVersion               string
	DeploymentEnvironment        string
	OTelTracesEnabled            bool
	OTelExporterOTLPEndpoint     string
	PyroscopeEnabled             bool
	PyroscopeServerAddress       string
	PyroscopeApplicationName     string
	PyroscopeUploadRate          time.Duration
	PyroscopeMutexBlockProfiling bool
}

type Runtime struct {
	tracerProvider *sdktrace.TracerProvider
	profiler       *pyroscope.Profiler
	logger         *slog.Logger
}

const defaultServiceName = "cets-api"

func StartRuntime(ctx context.Context, cfg RuntimeConfig, logger *slog.Logger) (*Runtime, error) {
	if logger == nil {
		logger = slog.Default()
	}
	runtimeObs := &Runtime{logger: logger}
	if err := runtimeObs.startTracing(ctx, cfg); err != nil {
		return nil, err
	}
	if err := runtimeObs.startProfiling(cfg); err != nil {
		_ = runtimeObs.Shutdown(ctx)
		return nil, err
	}
	return runtimeObs, nil
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	var joined error
	if r == nil {
		return nil
	}
	if r.profiler != nil {
		if err := r.profiler.Stop(); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	if r.tracerProvider != nil {
		if err := r.tracerProvider.Shutdown(ctx); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	return joined
}

func (r *Runtime) startTracing(ctx context.Context, cfg RuntimeConfig) error {
	if !cfg.OTelTracesEnabled {
		return nil
	}
	endpoint := strings.TrimSpace(cfg.OTelExporterOTLPEndpoint)
	if endpoint == "" {
		return errors.New("OTEL_EXPORTER_OTLP_ENDPOINT is required when OTEL_TRACES_ENABLED=true")
	}
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return err
	}
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(nonEmpty(cfg.ServiceName, defaultServiceName)),
		semconv.ServiceVersion(nonEmpty(cfg.ServiceVersion, "dev")),
		semconv.DeploymentEnvironmentName(nonEmpty(cfg.DeploymentEnvironment, "local")),
	)
	r.tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(r.tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	r.logger.Info("opentelemetry tracing enabled", "service", nonEmpty(cfg.ServiceName, defaultServiceName))
	return nil
}

func (r *Runtime) startProfiling(cfg RuntimeConfig) error {
	if !cfg.PyroscopeEnabled {
		return nil
	}
	if strings.TrimSpace(cfg.PyroscopeServerAddress) == "" {
		return errors.New("PYROSCOPE_SERVER_ADDRESS is required when PYROSCOPE_ENABLED=true")
	}
	if cfg.PyroscopeMutexBlockProfiling {
		runtime.SetMutexProfileFraction(5)
		runtime.SetBlockProfileRate(5)
	}
	uploadRate := cfg.PyroscopeUploadRate
	if uploadRate <= 0 {
		uploadRate = 15 * time.Second
	}
	profiler, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: nonEmpty(cfg.PyroscopeApplicationName, nonEmpty(cfg.ServiceName, defaultServiceName)),
		ServerAddress:   strings.TrimSpace(cfg.PyroscopeServerAddress),
		UploadRate:      uploadRate,
		Logger:          pyroscope.StandardLogger,
		Tags: map[string]string{
			"deployment_environment": nonEmpty(cfg.DeploymentEnvironment, "local"),
			"service_version":        nonEmpty(cfg.ServiceVersion, "dev"),
		},
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
	if err != nil {
		return err
	}
	r.profiler = profiler
	r.logger.Info("pyroscope profiling enabled", "application", nonEmpty(cfg.PyroscopeApplicationName, nonEmpty(cfg.ServiceName, defaultServiceName)))
	return nil
}

type RoutePatternFunc func(*http.Request) string

func TraceHTTP(routePattern RoutePatternFunc, next http.Handler) http.Handler {
	tracer := otel.Tracer("event-ticket-system/http")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, "HTTP "+r.Method, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		recorder := &traceStatusRecorder{ResponseWriter: w, status: http.StatusOK}
		tracedRequest := r.WithContext(ctx)
		next.ServeHTTP(recorder, tracedRequest)

		route := "/unknown"
		if routePattern != nil {
			route = nonEmpty(routePattern(tracedRequest), "/unknown")
		}
		span.SetName(r.Method + " " + route)
		span.SetAttributes(
			semconv.HTTPRequestMethodOriginal(r.Method),
			semconv.HTTPResponseStatusCode(recorder.status),
			semconv.HTTPRoute(route),
			attribute.String("cets.route", route),
		)
		if recorder.status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(recorder.status))
		}
	})
}

type traceStatusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *traceStatusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func nonEmpty(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
