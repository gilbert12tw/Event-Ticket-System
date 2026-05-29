package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaultsAndEnv(t *testing.T) {
	t.Setenv("APP_ADDR", ":9090")
	t.Setenv("APP_ENV", "test")
	t.Setenv("AUTH_MODE", "external_sso")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/cets")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("QUEUE_URL", "redis://localhost:6379/1")
	t.Setenv("OBJECT_STORAGE_ENDPOINT", "http://localhost:9000")
	t.Setenv("OBJECT_STORAGE_BUCKET", "cets-dev")
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY", "minioadmin")
	t.Setenv("OBJECT_STORAGE_SECRET_KEY", "minioadmin_dev_password")
	t.Setenv("MAILER_HOST", "mailhog")
	t.Setenv("MAILER_PORT", "1025")
	t.Setenv("MAILER_FROM", "tickets@example.test")
	t.Setenv("MAILER_REDIRECT_TO", "notifications@example.test")
	t.Setenv("TOKEN_SIGNING_SECRET", "test-secret")
	t.Setenv("PROVIDER_TOKEN_SECRET", "provider-test-secret")
	t.Setenv("AUTO_MIGRATE", "true")
	t.Setenv("REQUEST_TIMEOUT_MS", "2500")
	t.Setenv("DATABASE_TIMEOUT_MS", "1500")
	t.Setenv("SHUTDOWN_TIMEOUT_MS", "7500")
	t.Setenv("WORKER_POLL_INTERVAL_MS", "1250")
	t.Setenv("WORKER_MAX_ATTEMPTS", "5")
	t.Setenv("WORKER_BATCH_SIZE", "17")
	t.Setenv("NO_SHOW_THRESHOLD", "2")
	t.Setenv("NO_SHOW_COOLDOWN_DAYS", "45")
	t.Setenv("NO_SHOW_GRACE_HOURS", "12")
	t.Setenv("REPORT_STALE_THRESHOLD_SECONDS", "90")
	t.Setenv("REPORT_UNAVAILABLE_TIMEOUT_SECONDS", "240")
	t.Setenv("BOOKING_PREADMISSION", "on")
	t.Setenv("REDIS_OUTAGE_MODE", "fail")
	t.Setenv("RESERVATION_TTL_SECONDS", "30")
	t.Setenv("RESERVATION_TTL_GRACE_SECONDS", "15")
	t.Setenv("REDIS_OPERATION_TIMEOUT_MS", "175")
	t.Setenv("BOOKING_RESERVATION_HASH_SECRET", "reservation-hash-secret")

	cfg := Load()

	assert.Equal(t, ":9090", cfg.AppAddr)
	assert.Equal(t, "test", cfg.AppEnv)
	assert.Equal(t, "external_sso", cfg.AuthMode)
	assert.NotEmpty(t, cfg.DatabaseURL, "DatabaseURL was not loaded")
	assert.Equal(t, "redis://localhost:6379/0", cfg.RedisURL)
	assert.Equal(t, "redis://localhost:6379/1", cfg.QueueURL)
	assert.Equal(t, "http://localhost:9000", cfg.ObjectEndpoint)
	assert.Equal(t, "cets-dev", cfg.ObjectBucket)
	assert.Equal(t, "mailhog", cfg.MailerHost)
	assert.Equal(t, 1025, cfg.MailerPort)
	assert.Equal(t, "tickets@example.test", cfg.MailerFrom)
	assert.Equal(t, "notifications@example.test", cfg.MailerRedirectTo)
	assert.Equal(t, "test-secret", cfg.TokenSigningSecret)
	assert.Equal(t, "provider-test-secret", cfg.ProviderTokenSecret)
	assert.True(t, cfg.AutoMigrate)
	assert.Equal(t, 2500*time.Millisecond, cfg.RequestTimeout)
	assert.Equal(t, 1500*time.Millisecond, cfg.DatabaseTimeout)
	assert.Equal(t, 7500*time.Millisecond, cfg.ShutdownTimeout)
	assert.Equal(t, 1250*time.Millisecond, cfg.WorkerPollInterval)
	assert.Equal(t, 5, cfg.WorkerMaxAttempts)
	assert.Equal(t, 17, cfg.WorkerBatchSize)
	assert.Equal(t, 2, cfg.NoShowThreshold)
	assert.Equal(t, 45, cfg.NoShowCooldownDays)
	assert.Equal(t, 12, cfg.NoShowGraceHours)
	assert.Equal(t, 90, cfg.ReportStaleThresholdSeconds)
	assert.Equal(t, 240, cfg.ReportUnavailableTimeoutSeconds)
	assert.True(t, cfg.BookingPreadmission)
	assert.Equal(t, "fail", cfg.ReservationOutageMode)
	assert.Equal(t, 30*time.Second, cfg.ReservationTTL)
	assert.Equal(t, 15*time.Second, cfg.ReservationGraceTTL)
	assert.Equal(t, 175*time.Millisecond, cfg.ReservationOperationTimeout)
	assert.Equal(t, "reservation-hash-secret", cfg.BookingReservationHashSecret)
}

func TestValidateForServeRequiresDatabaseURL(t *testing.T) {
	cfg := Config{
		AppAddr:         ":8080",
		AppEnv:          "test",
		RequestTimeout:  time.Second,
		DatabaseTimeout: time.Second,
		ShutdownTimeout: time.Second,
	}

	require.Error(t, cfg.ValidateForServe(), "expected missing database URL error")
}

func TestLoadedConfigRejectsMalformedProductionValues(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("AUTH_MODE", "external_sso")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/cets")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("QUEUE_URL", "redis://localhost:6379/1")
	t.Setenv("OBJECT_STORAGE_ENDPOINT", "https://object-storage.example.test")
	t.Setenv("OBJECT_STORAGE_BUCKET", "cets-prod")
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY", "prod-object-access-key")
	t.Setenv("OBJECT_STORAGE_SECRET_KEY", "prod-object-secret-key")
	t.Setenv("MAILER_HOST", "smtp.example.test")
	t.Setenv("MAILER_FROM", "tickets@example.test")
	t.Setenv("TOKEN_SIGNING_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("PROVIDER_TOKEN_SECRET", "provider0123456789abcdef0123456789")
	t.Setenv("REQUEST_TIMEOUT_MS", "slow")
	t.Setenv("WORKER_MAX_ATTEMPTS", "0")

	err := Load().ValidateForServe()
	require.Error(t, err, "expected malformed env validation error")
	assert.Contains(t, err.Error(), "REQUEST_TIMEOUT_MS", "validation error did not include malformed keys")
}

func TestValidateForServeRequiresProductionSecret(t *testing.T) {
	cfg := productionServeConfig()
	cfg.TokenSigningSecret = localTokenSecret

	require.Error(t, cfg.ValidateForServe(), "expected production token secret error")
}

func TestValidateForServeRejectsDemoSecretInProduction(t *testing.T) {
	cfg := productionServeConfig()
	cfg.TokenSigningSecret = demoTokenSecret

	require.Error(t, cfg.ValidateForServe(), "expected production demo secret error")
}

func TestValidateForServeRejectsLocalAuthModeInProduction(t *testing.T) {
	cfg := productionServeConfig()
	cfg.AuthMode = "local_sso"

	require.Error(t, cfg.ValidateForServe(), "expected production auth mode error")
}

func TestValidateForServeRequiresProductionProviderTokenSecret(t *testing.T) {
	cfg := productionServeConfig()
	cfg.ProviderTokenSecret = demoProviderSecret

	require.Error(t, cfg.ValidateForServe(), "expected production provider token secret error")
}

func TestValidateForServeRequiresProductionBackingServices(t *testing.T) {
	cfg := productionServeConfig()
	cfg.RedisURL = ""

	require.Error(t, cfg.ValidateForServe(), "expected production Redis URL error")
}

func TestValidateForServeAcceptsProductionShape(t *testing.T) {
	require.NoError(t, productionServeConfig().ValidateForServe())
}

func TestValidateWorkerRequiresMailerAndPositiveRetryConfig(t *testing.T) {
	cfg := Config{
		DatabaseURL:        "postgres://user:pass@localhost:5432/cets",
		DatabaseTimeout:    time.Second,
		WorkerPollInterval: time.Second,
		WorkerMaxAttempts:  3,
		WorkerBatchSize:    25,
		MailerHost:         "mailhog",
		MailerPort:         1025,
		MailerFrom:         "tickets@example.test",
	}

	require.NoError(t, cfg.ValidateWorker())
	cfg.WorkerMaxAttempts = 0
	require.Error(t, cfg.ValidateWorker(), "expected worker max attempts validation error")
	cfg.WorkerMaxAttempts = 3
	cfg.WorkerBatchSize = 0
	require.Error(t, cfg.ValidateWorker(), "expected worker batch size validation error")
}

func TestValidateWorkerRejectsProductionDemoToken(t *testing.T) {
	cfg := productionServeConfig()
	cfg.WorkerPollInterval = time.Second
	cfg.WorkerMaxAttempts = 3
	cfg.WorkerBatchSize = 25
	cfg.TokenSigningSecret = demoTokenSecret

	require.Error(t, cfg.ValidateWorker(), "expected production worker token secret error")
}

func TestValidateWorkerRequiresProductionBackingServices(t *testing.T) {
	cfg := productionServeConfig()
	cfg.WorkerPollInterval = time.Second
	cfg.WorkerMaxAttempts = 3
	cfg.WorkerBatchSize = 25
	cfg.QueueURL = ""

	require.Error(t, cfg.ValidateWorker(), "expected production worker queue URL error")
}

func TestRedactedDatabaseURL(t *testing.T) {
	got := RedactedDatabaseURL("postgres://user:pass@localhost:5432/cets")
	assert.Equal(t, "postgres://***:***@localhost:5432/cets", got)
}

func productionServeConfig() Config {
	return Config{
		AppAddr:             ":8080",
		AppEnv:              "production",
		AuthMode:            "external_sso",
		DatabaseURL:         "postgres://user:pass@localhost:5432/cets",
		RedisURL:            "redis://localhost:6379/0",
		QueueURL:            "redis://localhost:6379/1",
		ObjectEndpoint:      "https://object-storage.example.test",
		ObjectBucket:        "cets-prod",
		ObjectRegion:        "us-east-1",
		ObjectAccessKey:     "prod-object-access-key",
		ObjectSecretKey:     "prod-object-secret-key",
		MailerHost:          "smtp.example.test",
		MailerPort:          587,
		MailerFrom:          "tickets@example.test",
		TokenSigningSecret:  "0123456789abcdef0123456789abcdef",
		ProviderTokenSecret: "provider0123456789abcdef0123456789",
		RequestTimeout:      time.Second,
		DatabaseTimeout:     time.Second,
		ShutdownTimeout:     time.Second,
	}
}
