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
	"event-ticket-system/internal/postgres"
	"event-ticket-system/internal/reservation"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func serve(cfg config.Config, logger *slog.Logger) error {
	return withDatabase(cfg, cfg.ValidateForServe, func(ctx context.Context, pool *pgxpool.Pool) error {
		if cfg.AutoMigrate {
			if err := postgres.Migrate(ctx, pool); err != nil {
				return err
			}
			logger.Info("migration complete", "mode", "auto")
		}

		gate, redisClient, err := newBookingReservationGate(cfg, logger)
		if err != nil {
			return err
		}
		if redisClient != nil {
			defer func() {
				if err := redisClient.Close(); err != nil {
					logger.Warn("redis client close failed", "error", err)
				}
			}()
		}
		ticketingService := newTicketingService(pool, cfg, logger).
			WithReservationGate(gate, []byte(cfg.BookingReservationHashSecret))

		router := httpapi.NewRouter(httpapi.Dependencies{
			DB:             pool,
			Ticketing:      ticketingService,
			Logger:         logger,
			RequestTimeout: cfg.RequestTimeout,
			AppEnv:         cfg.AppEnv,
			OpsAPIEnabled:  cfg.OpsAPIEnabled,
			ProviderAuth: httpapi.ProviderAuthConfig{
				Secret: cfg.ProviderTokenSecret,
			},
			ReportStaleThresholdSeconds: cfg.ReportStaleThresholdSeconds,
		})

		server := &http.Server{
			Addr:              cfg.AppAddr,
			Handler:           router,
			ReadHeaderTimeout: 5 * time.Second,
		}

		errCh := make(chan error, 1)
		go func() {
			logger.Info("http server listening", "addr", cfg.AppAddr)
			if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()

		stopCh := make(chan os.Signal, 1)
		signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

		select {
		case err := <-errCh:
			return err
		case sig := <-stopCh:
			logger.Info("shutdown signal received", "signal", sig.String())
		}

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer shutdownCancel()
		return server.Shutdown(shutdownCtx)
	})
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
