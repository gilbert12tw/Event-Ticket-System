package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/httpapi"
	"event-ticket-system/internal/observability"
	"event-ticket-system/internal/postgres"
	"event-ticket-system/internal/reservation"
	"event-ticket-system/internal/ticketing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type runtimeObservabilityServer interface {
	Shutdown(context.Context) error
}

func serve(cfg config.Config, logger *slog.Logger) error {
	runtimeObs, err := startRuntimeObservability(cfg, logger)
	if err != nil {
		return err
	}
	defer shutdownRuntimeObservability(cfg, logger, runtimeObs)

	return withDatabase(cfg, cfg.ValidateForServe, func(ctx context.Context, pool *pgxpool.Pool) error {
		return serveWithDatabase(ctx, cfg, logger, pool)
	})
}

func shutdownRuntimeObservability(cfg config.Config, logger *slog.Logger, runtimeObs runtimeObservabilityServer) {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := runtimeObs.Shutdown(shutdownCtx); err != nil {
		logger.Error("runtime observability shutdown failed", "error", err)
	}
}

func serveWithDatabase(ctx context.Context, cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool) error {
	if err := autoMigrateIfEnabled(ctx, cfg, logger, pool); err != nil {
		return err
	}
	readPool, err := connectReadPool(ctx, cfg, logger, pool)
	if err != nil {
		return err
	}
	if readPool != pool {
		defer readPool.Close()
	}
	gate, redisClient, err := newBookingReservationGate(cfg, logger)
	if err != nil {
		return err
	}
	defer closeRedisClient(logger, redisClient)

	metrics := observability.NewRegistry()
	ticketingService := newTicketingService(pool, cfg, logger).
		WithReservationGate(gate, []byte(cfg.BookingReservationHashSecret)).
		WithReservationOutageMode(cfg.ReservationOutageMode).
		WithMetrics(metrics)
	readTicketingService := newTicketingService(readPool, cfg, logger).
		WithMetrics(metrics)
	demoClock := newDemoClockForConfig(cfg, logger)
	if demoClock != nil {
		ticketingService.WithClock(demoClock.Now)
		readTicketingService.WithClock(demoClock.Now)
	}
	router := httpapi.NewRouter(httpapi.Dependencies{
		DB:                          pool,
		MetricsDB:                   observability.DatabaseMetrics{Write: pool, Read: readPool},
		Ticketing:                   ticketingService,
		ReadTicketing:               readTicketingService,
		Logger:                      logger,
		Metrics:                     metrics,
		DemoClock:                   demoClock,
		TracingEnabled:              cfg.OTelTracesEnabled,
		RequestTimeout:              cfg.RequestTimeout,
		AppEnv:                      cfg.AppEnv,
		OpsAPIEnabled:               cfg.OpsAPIEnabled,
		ProviderAuth:                httpapi.ProviderAuthConfig{Secret: cfg.ProviderTokenSecret},
		ReportStaleThresholdSeconds: cfg.ReportStaleThresholdSeconds,
	})
	server := &http.Server{Addr: cfg.AppAddr, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	return runHTTPServer(server, cfg, logger)
}

func connectReadPool(ctx context.Context, cfg config.Config, logger *slog.Logger, fallback *pgxpool.Pool) (*pgxpool.Pool, error) {
	readURL := strings.TrimSpace(cfg.DatabaseReadURL)
	if readURL == "" || readURL == strings.TrimSpace(cfg.DatabaseURL) {
		return fallback, nil
	}
	pool, err := postgres.Connect(ctx, readURL)
	if err != nil {
		return nil, fmt.Errorf("connect read database: %w", err)
	}
	if logger != nil {
		logger.Info("read database pool connected")
	}
	return pool, nil
}

func autoMigrateIfEnabled(ctx context.Context, cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool) error {
	if !cfg.AutoMigrate {
		return nil
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		return err
	}
	logger.Info("migration complete", "mode", "auto")
	return nil
}

func closeRedisClient(logger *slog.Logger, redisClient *redis.Client) {
	if redisClient == nil {
		return
	}
	if err := redisClient.Close(); err != nil {
		logger.Warn("redis client close failed", "error", err)
	}
}

func runHTTPServer(server *http.Server, cfg config.Config, logger *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.AppAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stopCh)

	select {
	case err := <-errCh:
		return err
	case sig := <-stopCh:
		logger.Info("shutdown signal received", "signal", sig.String())
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	return server.Shutdown(shutdownCtx)
}

func newDemoClockForConfig(cfg config.Config, logger *slog.Logger) *ticketing.DemoClock {
	if !cfg.DemoDebugEnabled {
		return nil
	}
	if logger != nil {
		logger.Info("demo debug clock enabled")
	}
	return ticketing.NewDemoClock()
}

// newBookingReservationGate constructs the PH2-22 Redis pre-admission gate.
// When BOOKING_PREADMISSION=off (default), returns reservation.NoopGate and a
// nil client so the booking path stays Phase 1 DB-only.
func newBookingReservationGate(cfg config.Config, logger *slog.Logger) (reservation.Gate, *redis.Client, error) {
	if !cfg.BookingPreadmission {
		return reservation.NoopGate{}, nil, nil
	}
	outageMode, err := reservation.ParseOutageMode(cfg.ReservationOutageMode)
	if err != nil {
		return nil, nil, err
	}
	gateCfg := reservation.Config{
		Enabled:          true,
		OutageMode:       outageMode,
		HashSecret:       []byte(cfg.BookingReservationHashSecret),
		TTL:              cfg.ReservationTTL,
		GraceTTL:         cfg.ReservationGraceTTL,
		OperationTimeout: cfg.ReservationOperationTimeout,
	}
	if err := gateCfg.Validate(); err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(cfg.RedisURL) == "" {
		return nil, nil, errors.New("REDIS_URL is required when BOOKING_PREADMISSION=on")
	}
	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid REDIS_URL: %w", err)
	}
	client := redis.NewClient(opts)
	return reservation.NewRedisGate(client, gateCfg, logger), client, nil
}
