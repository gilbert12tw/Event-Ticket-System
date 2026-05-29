package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/httpapi"
	"event-ticket-system/internal/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
)

func serve(cfg config.Config, logger *slog.Logger) error {
	return withDatabase(cfg, cfg.ValidateForServe, func(ctx context.Context, pool *pgxpool.Pool) error {
		if cfg.AutoMigrate {
			if err := postgres.Migrate(ctx, pool); err != nil {
				return err
			}
			logger.Info("migration complete", "mode", "auto")
		}

		router := httpapi.NewRouter(httpapi.Dependencies{
			DB:             pool,
			Ticketing:      newTicketingService(pool, cfg, logger),
			Logger:         logger,
			RequestTimeout: cfg.RequestTimeout,
			AppEnv:         cfg.AppEnv,
			OpsAPIEnabled:  cfg.OpsAPIEnabled,
			ProviderAuth: httpapi.ProviderAuthConfig{
				Secret: cfg.ProviderTokenSecret,
			},
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
