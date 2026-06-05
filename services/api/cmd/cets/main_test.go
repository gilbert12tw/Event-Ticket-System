package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"event-ticket-system/internal/config"
	"event-ticket-system/internal/httpapi"
	"event-ticket-system/internal/ticketing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run([]string{"unknown"}, testLogger())

	require.ErrorContains(t, err, "unknown command")
	require.NotContains(t, err.Error(), `"unknown"`)
}

func TestRunRejectsUnsafeUnknownCommandWithoutEcho(t *testing.T) {
	err := run([]string{"e1001@cets.local"}, testLogger())

	require.ErrorContains(t, err, "unknown command")
	require.NotContains(t, err.Error(), "e1001@cets.local")
}

func TestRunCommandsRequireDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")

	tests := []struct {
		name string
		args []string
	}{
		{name: "default serve"},
		{name: "serve", args: []string{"serve"}},
		{name: "ready", args: []string{"ready"}},
		{name: "migrate", args: []string{"migrate"}},
		{name: "reset demo db", args: []string{"reset-demo-db"}},
		{name: "seed", args: []string{"seed"}},
		{name: "worker", args: []string{"worker"}},
		{name: "hr sync", args: []string{"hr-sync"}},
		{name: "process no shows", args: []string{"process-no-shows"}},
		{name: "ops replay", args: []string{"ops", "replay", "--kind=notification", "--from=2026-05-28T10:00:00Z", "--to=2026-05-28T11:00:00Z"}},
		{name: "ops outbox stats", args: []string{"ops", "outbox-stats"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args, testLogger())

			require.ErrorContains(t, err, "DATABASE_URL is required")
		})
	}
}

func TestRunWorkerRejectsInvalidKindBeforeDatabaseConnect(t *testing.T) {
	t.Setenv("DATABASE_URL", "://invalid")

	err := run([]string{"worker", "--kinds=unknown"}, testLogger())

	require.ErrorContains(t, err, "WORKER_KINDS")
}

func TestRunWorkerRejectsUnsafeInvalidKindWithoutEcho(t *testing.T) {
	t.Setenv("DATABASE_URL", "://invalid")

	err := run([]string{"worker", "--kinds=e1001@cets.local"}, testLogger())

	require.ErrorContains(t, err, "WORKER_KINDS")
	require.NotContains(t, err.Error(), "e1001@cets.local")
	require.NotContains(t, err.Error(), "invalid database URL")
}

func TestRunWorkerRejectsUnknownWorkerFlag(t *testing.T) {
	t.Setenv("DATABASE_URL", "://invalid")

	err := run([]string{"worker", "--workers=projection"}, testLogger())

	require.ErrorContains(t, err, "unknown worker argument")
}

func TestRunWorkerRejectsMissingKindsValueBeforeDatabaseConnect(t *testing.T) {
	t.Setenv("DATABASE_URL", "://invalid")

	err := run([]string{"worker", "--kinds"}, testLogger())

	require.ErrorContains(t, err, `worker argument "--kinds" requires a value`)
}

func TestRunWorkerRejectsInvalidConcurrencyBeforeDatabaseConnect(t *testing.T) {
	t.Setenv("DATABASE_URL", "://invalid")
	t.Setenv("WORKER_CONCURRENCY_NOTIFICATION", "0")

	err := run([]string{"worker"}, testLogger())

	require.ErrorContains(t, err, "WORKER_CONCURRENCY_NOTIFICATION")
	require.NotContains(t, err.Error(), "invalid database URL")
}

func TestOpsReplayArgsDefaultDryRunAndApply(t *testing.T) {
	req, actor, err := parseOpsReplayArgs([]string{"--kind=notification", "--from=2026-05-28T10:00:00Z", "--to=2026-05-28T11:00:00Z"})

	require.NoError(t, err)
	require.Equal(t, "notification", req.Kind)
	require.Equal(t, time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC), req.From)
	require.Equal(t, time.Date(2026, 5, 28, 11, 0, 0, 0, time.UTC), req.To)
	require.True(t, req.DryRun)
	require.Equal(t, ticketing.Actor{ID: "ops-replay", Role: ticketing.RoleSystemAdmin}, actor)

	applyReq, applyActor, err := parseOpsReplayArgs([]string{
		"--kind", "notification",
		"--from", "2026-05-28T10:00:00Z",
		"--to", "2026-05-28T11:00:00Z",
		"--event-type", "notification.requested.v2",
		"--event-types", "booking.confirmed, booking.waitlisted",
		"--actor-id", "ops-admin-1",
		"--apply",
	})

	require.NoError(t, err)
	require.False(t, applyReq.DryRun)
	require.Equal(t, []string{"notification.requested.v2", "booking.confirmed", "booking.waitlisted"}, applyReq.EventTypes)
	require.Equal(t, ticketing.Actor{ID: "ops-admin-1", Role: ticketing.RoleSystemAdmin}, applyActor)
}

func TestOpsReplayArgsRejectsInvalidValues(t *testing.T) {
	_, _, err := parseOpsReplayArgs([]string{"--unknown"})
	require.ErrorContains(t, err, "unknown ops replay argument")

	_, _, err = parseOpsReplayArgs([]string{"--kind=notification", "--from=not-time"})
	require.ErrorContains(t, err, "invalid replay from time")

	_, _, err = parseOpsReplayArgs([]string{"--kind=notification", "--to=not-time"})
	require.ErrorContains(t, err, "invalid replay to time")

	_, _, err = parseOpsReplayArgs([]string{
		"--kind=notification",
		"--from=2026-05-28T10:00:00Z",
		"--to=2026-05-28T11:00:00Z",
		"--actor-id", " ",
	})
	require.ErrorContains(t, err, "actor id")
}

func TestOpsReplayArgsLastDryRunFlagWins(t *testing.T) {
	req, _, err := parseOpsReplayArgs([]string{
		"--kind=notification",
		"--from=2026-05-28T10:00:00Z",
		"--to=2026-05-28T11:00:00Z",
		"--apply",
		"--dry-run",
	})

	require.NoError(t, err)
	require.True(t, req.DryRun)
}

func TestOutboxStatsLastProcessedFormatsUTC(t *testing.T) {
	require.Empty(t, outboxStatsLastProcessed(ticketing.OutboxQueueStatusRow{}))

	when := time.Date(2026, 5, 28, 10, 0, 0, 0, time.FixedZone("TST", 8*60*60))
	require.Equal(t, "2026-05-28T02:00:00Z", outboxStatsLastProcessed(ticketing.OutboxQueueStatusRow{LastProcessedAt: &when}))
}

func TestNewBookingReservationGateValidation(t *testing.T) {
	cfg := config.Config{
		BookingReservationHashSecret: "reservation-hash-secret",
		ReservationOutageMode:        "degrade",
		ReservationTTL:               20 * time.Second,
		ReservationGraceTTL:          5 * time.Second,
		ReservationOperationTimeout:  150 * time.Millisecond,
	}
	cfg.BookingPreadmission = false
	gate, client, err := newBookingReservationGate(cfg, testLogger())
	require.NoError(t, err)
	require.Nil(t, client)
	require.NotNil(t, gate)

	cfg.BookingPreadmission = true
	cfg.ReservationOutageMode = "invalid"
	_, _, err = newBookingReservationGate(cfg, testLogger())
	require.Error(t, err)

	cfg.ReservationOutageMode = "degrade"
	cfg.RedisURL = ""
	_, _, err = newBookingReservationGate(cfg, testLogger())
	require.ErrorContains(t, err, "REDIS_URL")

	cfg.RedisURL = "://bad"
	_, _, err = newBookingReservationGate(cfg, testLogger())
	require.ErrorContains(t, err, "invalid REDIS_URL")
}

func TestNewBookingRateLimiterValidation(t *testing.T) {
	cfg := config.Config{
		RateLimitEnabled:            false,
		BookingRateLimitPerActor:    1,
		RateLimitOutageMode:         "degrade",
		ReservationOperationTimeout: 150 * time.Millisecond,
		BookingRateLimitHashSecret:  "rate-limit-hash-secret",
	}
	limiter, client, err := newBookingRateLimiter(cfg, testLogger())
	require.NoError(t, err)
	require.Nil(t, client)
	require.NotNil(t, limiter)

	cfg.RateLimitEnabled = true
	cfg.RateLimitOutageMode = "invalid"
	_, _, err = newBookingRateLimiter(cfg, testLogger())
	require.ErrorContains(t, err, "RATE_LIMIT_OUTAGE_MODE")

	cfg.RateLimitOutageMode = "degrade"
	cfg.BookingRateLimitHashSecret = ""
	_, _, err = newBookingRateLimiter(cfg, testLogger())
	require.ErrorContains(t, err, "BOOKING_RATE_LIMIT_HASH_SECRET")

	cfg.BookingRateLimitHashSecret = "rate-limit-hash-secret"
	cfg.RedisURL = ""
	_, _, err = newBookingRateLimiter(cfg, testLogger())
	require.ErrorContains(t, err, "REDIS_URL")

	cfg.RedisURL = "://bad"
	_, _, err = newBookingRateLimiter(cfg, testLogger())
	require.ErrorContains(t, err, "invalid REDIS_URL")
}

func TestRunOpsReplayRejectsInvalidKindBeforeDatabaseConnect(t *testing.T) {
	t.Setenv("DATABASE_URL", "://invalid")

	err := run([]string{"ops", "replay", "--kind=unknown", "--from=2026-05-28T10:00:00Z", "--to=2026-05-28T11:00:00Z"}, testLogger())

	require.ErrorContains(t, err, "worker kind")
}

func TestRunOpsOutboxStatsRejectsUnknownArgBeforeDatabaseConnect(t *testing.T) {
	t.Setenv("DATABASE_URL", "://invalid")

	err := run([]string{"ops", "outbox-stats", "--kind=notification"}, testLogger())

	require.ErrorContains(t, err, "unknown ops outbox-stats argument")
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
		{name: "reset demo db", run: func(cfg config.Config) error {
			cfg.RedisURL = "redis://localhost:6379/0"
			return resetDemoDB(cfg, testLogger())
		}},
		{name: "seed", run: func(cfg config.Config) error { return seed(cfg, testLogger()) }},
		{name: "worker", run: func(cfg config.Config) error { return worker(cfg, testLogger(), nil) }},
		{name: "hr sync", run: func(cfg config.Config) error { return hrSync(cfg, testLogger(), []string{"scheduled import"}) }},
		{name: "process no shows", run: func(cfg config.Config) error { return processNoShows(cfg, testLogger()) }},
		{name: "ops replay", run: func(cfg config.Config) error {
			return ops(cfg, testLogger(), []string{"replay", "--kind=notification", "--from=2026-05-28T10:00:00Z", "--to=2026-05-28T11:00:00Z"})
		}},
		{name: "ops outbox stats", run: func(cfg config.Config) error {
			return ops(cfg, testLogger(), []string{"outbox-stats"})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(cfg)

			require.Error(t, err)
		})
	}
}

func TestResetDemoDBRejectsMissingRedisURLBeforeDatabaseConnect(t *testing.T) {
	cfg := validCommandConfig()
	cfg.RedisURL = ""

	err := resetDemoDB(cfg, testLogger())

	require.ErrorContains(t, err, "REDIS_URL is required")
	require.NotContains(t, err.Error(), "invalid database URL")
}

func TestResetDemoDBRejectsInvalidRedisURLBeforeDatabaseConnect(t *testing.T) {
	cfg := validCommandConfig()
	cfg.RedisURL = "://invalid"

	err := resetDemoDB(cfg, testLogger())

	require.ErrorContains(t, err, "invalid REDIS_URL")
	require.NotContains(t, err.Error(), "invalid database URL")
}

func TestResetDemoDBRejectsProductionBeforeBackingServiceAccess(t *testing.T) {
	cfg := validCommandConfig()
	cfg.AppEnv = "production"
	cfg.RedisURL = "://invalid"

	err := resetDemoDB(cfg, testLogger())

	require.ErrorContains(t, err, "reset-demo-db is disabled when APP_ENV=production")
	require.NotContains(t, err.Error(), "invalid REDIS_URL")
	require.NotContains(t, err.Error(), "invalid database URL")
}

func TestNewTicketingServiceBuildsService(t *testing.T) {
	service := newTicketingService(nil, validCommandConfig(), nil)

	require.NotNil(t, service)
}

func TestDemoDebugConfigControlsDemoClockRouterWiring(t *testing.T) {
	cfg := validCommandConfig()
	cfg.AppEnv = "test"
	cfg.DemoDebugEnabled = true
	enabledClock := newDemoClockForConfig(cfg, testLogger())
	require.NotNil(t, enabledClock)
	enabledRouter := httpapi.NewRouter(httpapi.Dependencies{
		DemoClock:      enabledClock,
		RequestTimeout: time.Second,
		AppEnv:         cfg.AppEnv,
		ProviderAuth: httpapi.ProviderAuthConfig{
			Secret: cfg.ProviderTokenSecret,
		},
	})
	enabledToken := mockProviderBearerToken(t, enabledRouter, "admin-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/debug/demo-clock", nil)
	req.Header.Set("Authorization", "Bearer "+enabledToken)
	rec := httptest.NewRecorder()
	enabledRouter.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var enabledEnvelope struct {
		Data struct {
			Enabled bool `json:"enabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &enabledEnvelope))
	assert.True(t, enabledEnvelope.Data.Enabled)

	cfg.DemoDebugEnabled = false
	disabledClock := newDemoClockForConfig(cfg, testLogger())
	require.Nil(t, disabledClock)
	disabledRouter := httpapi.NewRouter(httpapi.Dependencies{
		DemoClock:      disabledClock,
		RequestTimeout: time.Second,
		AppEnv:         cfg.AppEnv,
		ProviderAuth: httpapi.ProviderAuthConfig{
			Secret: cfg.ProviderTokenSecret,
		},
	})
	disabledToken := mockProviderBearerToken(t, disabledRouter, "admin-1")
	disabledReq := httptest.NewRequest(http.MethodGet, "/api/v1/debug/demo-clock", nil)
	disabledReq.Header.Set("Authorization", "Bearer "+disabledToken)
	disabledRec := httptest.NewRecorder()
	disabledRouter.ServeHTTP(disabledRec, disabledReq)

	assert.Equal(t, http.StatusNotFound, disabledRec.Code)
}

func mockProviderBearerToken(t *testing.T, router http.Handler, profileID string) string {
	t.Helper()
	body := bytes.NewBufferString(`{"profile_id":"` + profileID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mock-provider-token", body)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var envelope struct {
		Data struct {
			ProviderToken string `json:"provider_token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.NotEmpty(t, envelope.Data.ProviderToken)
	return envelope.Data.ProviderToken
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
		WorkerShutdownGrace: 30 * time.Second,
		WorkerMaxAttempts:   3,
		WorkerBatchSize:     25,
		OutboxRetryMax:      10,
		OutboxBackoffBase:   500 * time.Millisecond,
		OutboxBackoffMax:    time.Minute,
		WorkerKinds:         []string{"notification", "projection", "compensation", "export"},
		WorkerConcurrency:   map[string]int{"notification": 4, "projection": 2, "compensation": 1, "export": 1},
		NoShowThreshold:     1,
		NoShowCooldownDays:  90,
		NoShowGraceHours:    24,
	}
}
