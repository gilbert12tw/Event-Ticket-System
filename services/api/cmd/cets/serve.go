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
	"event-ticket-system/internal/ticketing"
)

func serve(cfg config.Config, logger *slog.Logger) error {
	if err := cfg.ValidateForServe(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DatabaseTimeout)
	defer cancel()

	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		if err := postgres.Migrate(ctx, pool); err != nil {
			return err
		}
		logger.Info("migration complete", "mode", "auto")
	}

	router := httpapi.NewRouter(httpapi.Dependencies{
		DB:             pool,
		Ticketing:      ticketing.NewService(pool, ticketing.NewSigner(cfg.TokenSigningSecret), logger),
		Logger:         logger,
		RequestTimeout: cfg.RequestTimeout,
		AppEnv:         cfg.AppEnv,
		AuthSession: httpapi.AuthConfig{
			Secret:       cfg.AuthSessionSecret,
			TTL:          cfg.AuthSessionTTL,
			CookieSecure: cfg.AuthCookieSecure,
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
}
