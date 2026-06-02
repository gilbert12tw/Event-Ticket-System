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
	t.Setenv("OPS_API_ENABLED", "true")
	t.Setenv("DEMO_DEBUG_ENABLED", "true")
	t.Setenv("REQUEST_TIMEOUT_MS", "2500")
	t.Setenv("DATABASE_TIMEOUT_MS", "1500")
	t.Setenv("SHUTDOWN_TIMEOUT_MS", "7500")
	t.Setenv("WORKER_POLL_INTERVAL_MS", "1250")
	t.Setenv("WORKER_SHUTDOWN_GRACE_SECONDS", "12")
	t.Setenv("WORKER_MAX_ATTEMPTS", "5")
	t.Setenv("WORKER_BATCH_SIZE", "17")
	t.Setenv("OUTBOX_LEASE_TTL_SECONDS", "45")
	t.Setenv("OUTBOX_RETRY_MAX", "7")
	t.Setenv("OUTBOX_BACKOFF_BASE_MS", "250")
	t.Setenv("OUTBOX_BACKOFF_MAX_MS", "9000")
	t.Setenv("WORKER_KINDS", "notification,projection")
	t.Setenv("WORKER_CONCURRENCY_NOTIFICATION", "6")
	t.Setenv("WORKER_CONCURRENCY_PROJECTION", "3")
	t.Setenv("WORKER_CONCURRENCY_COMPENSATION", "2")
	t.Setenv("WORKER_CONCURRENCY_EXPORT", "1")
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
	assert.True(t, cfg.OpsAPIEnabled)
	assert.True(t, cfg.DemoDebugEnabled)
	assert.Equal(t, 2500*time.Millisecond, cfg.RequestTimeout)
	assert.Equal(t, 1500*time.Millisecond, cfg.DatabaseTimeout)
	assert.Equal(t, 7500*time.Millisecond, cfg.ShutdownTimeout)
	assert.Equal(t, 1250*time.Millisecond, cfg.WorkerPollInterval)
	assert.Equal(t, 12*time.Second, cfg.WorkerShutdownGrace)
	assert.Equal(t, 5, cfg.WorkerMaxAttempts)
	assert.Equal(t, 17, cfg.WorkerBatchSize)
	assert.Equal(t, 45*time.Second, cfg.OutboxLeaseTTL)
	assert.Equal(t, 7, cfg.OutboxRetryMax)
	assert.Equal(t, 250*time.Millisecond, cfg.OutboxBackoffBase)
	assert.Equal(t, 9*time.Second, cfg.OutboxBackoffMax)
	assert.Equal(t, []string{"notification", "projection"}, cfg.WorkerKinds)
	assert.Equal(t, 6, cfg.WorkerConcurrency["notification"])
	assert.Equal(t, 3, cfg.WorkerConcurrency["projection"])
	assert.Equal(t, 2, cfg.WorkerConcurrency["compensation"])
	assert.Equal(t, 1, cfg.WorkerConcurrency["export"])
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

func TestLoadDefaultsOutboxRetryPolicy(t *testing.T) {
	t.Setenv("OUTBOX_LEASE_TTL_SECONDS", "")
	t.Setenv("OUTBOX_RETRY_MAX", "")
	t.Setenv("OUTBOX_BACKOFF_BASE_MS", "")
	t.Setenv("OUTBOX_BACKOFF_MAX_MS", "")

	cfg := Load()

	assert.Equal(t, time.Minute, cfg.OutboxLeaseTTL)
	assert.Equal(t, 10, cfg.OutboxRetryMax)
	assert.Equal(t, 500*time.Millisecond, cfg.OutboxBackoffBase)
	assert.Equal(t, 60*time.Second, cfg.OutboxBackoffMax)
}

func TestLoadPrefersOutboxBatchSizeOverLegacyWorkerBatchSize(t *testing.T) {
	t.Setenv("OUTBOX_BATCH_SIZE", "41")
	t.Setenv("WORKER_BATCH_SIZE", "17")

	cfg := Load()

	assert.Equal(t, 41, cfg.WorkerBatchSize)
}

func TestLoadDefaultsOutboxBatchSize(t *testing.T) {
	t.Setenv("OUTBOX_BATCH_SIZE", "")
	t.Setenv("WORKER_BATCH_SIZE", "")

	cfg := Load()

	assert.Equal(t, 100, cfg.WorkerBatchSize)
}

func TestValidateWorkerAcceptsZeroOutboxRetryMax(t *testing.T) {
	cfg := validWorkerConfig()
	cfg.OutboxRetryMax = 0

	require.NoError(t, cfg.ValidateWorker())
}

func TestValidateWorkerRejectsInvalidOutboxBackoff(t *testing.T) {
	cfg := validWorkerConfig()
	cfg.OutboxBackoffBase = 0

	err := cfg.ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "OUTBOX_BACKOFF_BASE_MS")
	cfg.OutboxBackoffBase = time.Second
	cfg.OutboxBackoffMax = 500 * time.Millisecond
	err = cfg.ValidateWorker()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OUTBOX_BACKOFF_MAX_MS")
}

func TestLoadedConfigRejectsMalformedOutboxRetryPolicy(t *testing.T) {
	t.Setenv("OUTBOX_RETRY_MAX", "-1")
	t.Setenv("OUTBOX_BACKOFF_BASE_MS", "fast")

	err := Load().ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "OUTBOX_RETRY_MAX")
	assert.Contains(t, err.Error(), "OUTBOX_BACKOFF_BASE_MS")
}

func TestLoadedConfigRejectsMalformedOutboxLeaseTTL(t *testing.T) {
	t.Setenv("OUTBOX_LEASE_TTL_SECONDS", "0")

	err := Load().ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "OUTBOX_LEASE_TTL_SECONDS")
}

func TestLoadedConfigRejectsMalformedOutboxBatchSize(t *testing.T) {
	t.Setenv("OUTBOX_BATCH_SIZE", "0")

	err := Load().ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "OUTBOX_BATCH_SIZE")
}

func TestLoadDefaultsWorkerKindsAndConcurrency(t *testing.T) {
	t.Setenv("WORKER_KINDS", "")
	t.Setenv("WORKER_SHUTDOWN_GRACE_SECONDS", "")
	t.Setenv("WORKER_CONCURRENCY_NOTIFICATION", "")
	t.Setenv("WORKER_CONCURRENCY_PROJECTION", "")
	t.Setenv("WORKER_CONCURRENCY_COMPENSATION", "")
	t.Setenv("WORKER_CONCURRENCY_EXPORT", "")

	cfg := Load()

	assert.Equal(t, []string{"notification", "projection", "compensation", "export"}, cfg.WorkerKinds)
	assert.Equal(t, 30*time.Second, cfg.WorkerShutdownGrace)
	assert.Equal(t, 4, cfg.WorkerConcurrency["notification"])
	assert.Equal(t, 2, cfg.WorkerConcurrency["projection"])
	assert.Equal(t, 1, cfg.WorkerConcurrency["compensation"])
	assert.Equal(t, 1, cfg.WorkerConcurrency["export"])
}

func TestLoadExpandsAllWorkerKinds(t *testing.T) {
	t.Setenv("WORKER_KINDS", "*")

	cfg := Load()

	assert.Equal(t, []string{"notification", "projection", "compensation", "export"}, cfg.WorkerKinds)
}

func TestValidateWorkerRejectsInvalidWorkerKinds(t *testing.T) {
	cfg := validWorkerConfig()
	cfg.WorkerKinds = []string{"notification", "unknown"}

	err := cfg.ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKER_KINDS")
	assert.NotContains(t, err.Error(), "postgres://")
}

func TestValidateWorkerRejectsReplayOnlyWorkerKindAlias(t *testing.T) {
	cfg := validWorkerConfig()
	cfg.WorkerKinds = []string{"reservation_compensation"}

	err := cfg.ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKER_KINDS")
	assert.NotContains(t, err.Error(), "postgres://")
}

func TestValidateWorkerRejectsDuplicateWorkerKinds(t *testing.T) {
	cfg := validWorkerConfig()
	cfg.WorkerKinds = []string{"notification", "notification"}

	require.ErrorContains(t, cfg.ValidateWorker(), "WORKER_KINDS")
}

func TestValidateWorkerRejectsInvalidWorkerConcurrency(t *testing.T) {
	cfg := validWorkerConfig()
	cfg.WorkerConcurrency["notification"] = 0

	err := cfg.ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKER_CONCURRENCY_NOTIFICATION")
}

func TestValidateWorkerAcceptsZeroShutdownGrace(t *testing.T) {
	cfg := validWorkerConfig()
	cfg.WorkerShutdownGrace = 0

	require.NoError(t, cfg.ValidateWorker())
}

func TestLoadedConfigRejectsMalformedWorkerShutdownGrace(t *testing.T) {
	t.Setenv("WORKER_SHUTDOWN_GRACE_SECONDS", "-1")

	err := Load().ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKER_SHUTDOWN_GRACE_SECONDS")
}

func TestLoadedConfigRejectsMalformedWorkerConcurrency(t *testing.T) {
	t.Setenv("WORKER_CONCURRENCY_EXPORT", "fast")

	err := Load().ValidateWorker()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKER_CONCURRENCY_EXPORT")
}

func TestWorkerKindsOverrideArgs(t *testing.T) {
	cfg := validWorkerConfig()

	got, err := cfg.WithWorkerArgs([]string{"--kinds=projection,export"})

	require.NoError(t, err)
	assert.Equal(t, []string{"projection", "export"}, got.WorkerKinds)
}

func TestWorkerKindsOverrideArgsAcceptSeparateValue(t *testing.T) {
	cfg := validWorkerConfig()

	got, err := cfg.WithWorkerArgs([]string{"--kinds", "projection,export"})

	require.NoError(t, err)
	assert.Equal(t, []string{"projection", "export"}, got.WorkerKinds)
}

func TestWorkerKindsOverrideArgsRequireValue(t *testing.T) {
	cfg := validWorkerConfig()

	_, err := cfg.WithWorkerArgs([]string{"--kinds"})

	require.ErrorContains(t, err, `worker argument "--kinds" requires a value`)
}

func TestWorkerKindsOverrideArgsRejectUnknownFlag(t *testing.T) {
	cfg := validWorkerConfig()

	_, err := cfg.WithWorkerArgs([]string{"--workers=projection"})

	require.ErrorContains(t, err, "unknown worker argument")
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
	cfg := validWorkerConfig()

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
	cfg.OutboxLeaseTTL = time.Minute
	cfg.WorkerKinds = []string{"notification", "projection", "compensation", "export"}
	cfg.WorkerConcurrency = defaultWorkerConcurrency()
	cfg.TokenSigningSecret = demoTokenSecret

	require.Error(t, cfg.ValidateWorker(), "expected production worker token secret error")
}

func TestValidateWorkerRequiresProductionBackingServices(t *testing.T) {
	cfg := productionServeConfig()
	cfg.WorkerPollInterval = time.Second
	cfg.WorkerMaxAttempts = 3
	cfg.WorkerBatchSize = 25
	cfg.OutboxLeaseTTL = time.Minute
	cfg.WorkerKinds = []string{"notification", "projection", "compensation", "export"}
	cfg.WorkerConcurrency = defaultWorkerConcurrency()
	cfg.QueueURL = ""

	require.Error(t, cfg.ValidateWorker(), "expected production worker queue URL error")
}

func validWorkerConfig() Config {
	return Config{
		DatabaseURL:        "postgres://user:pass@localhost:5432/cets",
		DatabaseTimeout:    time.Second,
		WorkerPollInterval: time.Second,
		WorkerMaxAttempts:  3,
		WorkerBatchSize:    25,
		OutboxLeaseTTL:     time.Minute,
		OutboxRetryMax:     10,
		OutboxBackoffBase:  500 * time.Millisecond,
		OutboxBackoffMax:   time.Minute,
		WorkerKinds:        []string{"notification", "projection", "compensation", "export"},
		WorkerConcurrency:  defaultWorkerConcurrency(),
		MailerHost:         "mailhog",
		MailerPort:         1025,
		MailerFrom:         "tickets@example.test",
	}
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
		OutboxLeaseTTL:      time.Minute,
		OutboxRetryMax:      10,
		OutboxBackoffBase:   500 * time.Millisecond,
		OutboxBackoffMax:    time.Minute,
	}
}
