package main

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"event-ticket-system/internal/config"

	"github.com/stretchr/testify/require"
)

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run([]string{"unknown"}, testLogger())

	require.ErrorContains(t, err, `unknown command "unknown"`)
}

func TestRunCommandsRequireDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	tests := []struct {
		name string
		args []string
	}{
		{name: "default serve"},
		{name: "serve", args: []string{"serve"}},
		{name: "ready", args: []string{"ready"}},
		{name: "migrate", args: []string{"migrate"}},
		{name: "seed", args: []string{"seed"}},
		{name: "worker", args: []string{"worker"}},
		{name: "hr sync", args: []string{"hr-sync"}},
		{name: "process no shows", args: []string{"process-no-shows"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args, testLogger())

			require.ErrorContains(t, err, "DATABASE_URL is required")
		})
	}
}

func TestCommandsReturnDatabaseParseErrorsAfterValidation(t *testing.T) {
	cfg := validCommandConfig()

	tests := []struct {
		name string
		run  func(config.Config) error
	}{
		{name: "serve", run: func(cfg config.Config) error { return serve(cfg, testLogger()) }},
		{name: "ready", run: ready},
		{name: "migrate", run: func(cfg config.Config) error { return migrate(cfg, testLogger()) }},
		{name: "seed", run: func(cfg config.Config) error { return seed(cfg, testLogger()) }},
		{name: "worker", run: func(cfg config.Config) error { return worker(cfg, testLogger()) }},
		{name: "hr sync", run: func(cfg config.Config) error { return hrSync(cfg, testLogger(), []string{"scheduled import"}) }},
		{name: "process no shows", run: func(cfg config.Config) error { return processNoShows(cfg, testLogger()) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(cfg)

			require.Error(t, err)
		})
	}
}

func TestNewTicketingServiceBuildsService(t *testing.T) {
	service := newTicketingService(nil, validCommandConfig(), nil)

	require.NotNil(t, service)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func validCommandConfig() config.Config {
	return config.Config{
		AppAddr:             ":0",
		AppEnv:              "local",
		AuthMode:            "external_sso",
		DatabaseURL:         "://invalid",
		MailerHost:          "localhost",
		MailerPort:          1025,
		MailerFrom:          "no-reply@cets.local",
		TokenSigningSecret:  "local-dev-token-secret",
		ProviderTokenSecret: "local-dev-provider-token-secret",
		RequestTimeout:      time.Second,
		DatabaseTimeout:     time.Second,
		ShutdownTimeout:     time.Second,
		WorkerPollInterval:  time.Second,
		WorkerMaxAttempts:   3,
		WorkerBatchSize:     25,
		NoShowThreshold:     1,
		NoShowCooldownDays:  90,
		NoShowGraceHours:    24,
	}
}
