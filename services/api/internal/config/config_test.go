package config

import (
	"strings"
	"testing"
	"time"
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

	cfg := Load()

	if cfg.AppAddr != ":9090" {
		t.Fatalf("AppAddr = %q", cfg.AppAddr)
	}
	if cfg.AppEnv != "test" {
		t.Fatalf("AppEnv = %q", cfg.AppEnv)
	}
	if cfg.AuthMode != "external_sso" {
		t.Fatalf("AuthMode = %q", cfg.AuthMode)
	}
	if cfg.DatabaseURL == "" {
		t.Fatal("DatabaseURL was not loaded")
	}
	if cfg.RedisURL != "redis://localhost:6379/0" || cfg.QueueURL != "redis://localhost:6379/1" {
		t.Fatalf("Redis/Queue URLs were not loaded: %#v", cfg)
	}
	if cfg.ObjectEndpoint != "http://localhost:9000" || cfg.ObjectBucket != "cets-dev" {
		t.Fatalf("object storage config was not loaded: %#v", cfg)
	}
	if cfg.MailerHost != "mailhog" || cfg.MailerPort != 1025 || cfg.MailerFrom != "tickets@example.test" {
		t.Fatalf("mailer config was not loaded: %#v", cfg)
	}
	if cfg.MailerRedirectTo != "notifications@example.test" {
		t.Fatalf("MailerRedirectTo = %q", cfg.MailerRedirectTo)
	}
	if cfg.TokenSigningSecret != "test-secret" {
		t.Fatal("TokenSigningSecret was not loaded")
	}
	if cfg.ProviderTokenSecret != "provider-test-secret" {
		t.Fatal("ProviderTokenSecret was not loaded")
	}
	if !cfg.AutoMigrate {
		t.Fatal("AutoMigrate = false")
	}
	if cfg.RequestTimeout != 2500*time.Millisecond {
		t.Fatalf("RequestTimeout = %s", cfg.RequestTimeout)
	}
	if cfg.DatabaseTimeout != 1500*time.Millisecond {
		t.Fatalf("DatabaseTimeout = %s", cfg.DatabaseTimeout)
	}
	if cfg.ShutdownTimeout != 7500*time.Millisecond {
		t.Fatalf("ShutdownTimeout = %s", cfg.ShutdownTimeout)
	}
	if cfg.WorkerPollInterval != 1250*time.Millisecond || cfg.WorkerMaxAttempts != 5 || cfg.WorkerBatchSize != 17 {
		t.Fatalf("worker config = %s/%d/%d", cfg.WorkerPollInterval, cfg.WorkerMaxAttempts, cfg.WorkerBatchSize)
	}
}

func TestValidateForServeRequiresDatabaseURL(t *testing.T) {
	cfg := Config{
		AppAddr:         ":8080",
		AppEnv:          "test",
		RequestTimeout:  time.Second,
		DatabaseTimeout: time.Second,
		ShutdownTimeout: time.Second,
	}

	if err := cfg.ValidateForServe(); err == nil {
		t.Fatal("expected missing database URL error")
	}
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
	if err == nil {
		t.Fatal("expected malformed env validation error")
	}
	if !strings.Contains(err.Error(), "REQUEST_TIMEOUT_MS") {
		t.Fatalf("validation error did not include malformed keys: %v", err)
	}
}

func TestValidateForServeRequiresProductionSecret(t *testing.T) {
	cfg := productionServeConfig()
	cfg.TokenSigningSecret = localTokenSecret

	if err := cfg.ValidateForServe(); err == nil {
		t.Fatal("expected production token secret error")
	}
}

func TestValidateForServeRejectsDemoSecretInProduction(t *testing.T) {
	cfg := productionServeConfig()
	cfg.TokenSigningSecret = demoTokenSecret

	if err := cfg.ValidateForServe(); err == nil {
		t.Fatal("expected production demo secret error")
	}
}

func TestValidateForServeRejectsLocalAuthModeInProduction(t *testing.T) {
	cfg := productionServeConfig()
	cfg.AuthMode = "local_sso"

	if err := cfg.ValidateForServe(); err == nil {
		t.Fatal("expected production auth mode error")
	}
}

func TestValidateForServeRequiresProductionProviderTokenSecret(t *testing.T) {
	cfg := productionServeConfig()
	cfg.ProviderTokenSecret = demoProviderSecret

	if err := cfg.ValidateForServe(); err == nil {
		t.Fatal("expected production provider token secret error")
	}
}

func TestValidateForServeRequiresProductionBackingServices(t *testing.T) {
	cfg := productionServeConfig()
	cfg.RedisURL = ""

	if err := cfg.ValidateForServe(); err == nil {
		t.Fatal("expected production Redis URL error")
	}
}

func TestValidateForServeAcceptsProductionShape(t *testing.T) {
	if err := productionServeConfig().ValidateForServe(); err != nil {
		t.Fatalf("ValidateForServe returned error: %v", err)
	}
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

	if err := cfg.ValidateWorker(); err != nil {
		t.Fatalf("ValidateWorker returned error: %v", err)
	}
	cfg.WorkerMaxAttempts = 0
	if err := cfg.ValidateWorker(); err == nil {
		t.Fatal("expected worker max attempts validation error")
	}
	cfg.WorkerMaxAttempts = 3
	cfg.WorkerBatchSize = 0
	if err := cfg.ValidateWorker(); err == nil {
		t.Fatal("expected worker batch size validation error")
	}
}

func TestValidateWorkerRejectsProductionDemoToken(t *testing.T) {
	cfg := productionServeConfig()
	cfg.WorkerPollInterval = time.Second
	cfg.WorkerMaxAttempts = 3
	cfg.WorkerBatchSize = 25
	cfg.TokenSigningSecret = demoTokenSecret

	if err := cfg.ValidateWorker(); err == nil {
		t.Fatal("expected production worker token secret error")
	}
}

func TestValidateWorkerRequiresProductionBackingServices(t *testing.T) {
	cfg := productionServeConfig()
	cfg.WorkerPollInterval = time.Second
	cfg.WorkerMaxAttempts = 3
	cfg.WorkerBatchSize = 25
	cfg.QueueURL = ""

	if err := cfg.ValidateWorker(); err == nil {
		t.Fatal("expected production worker queue URL error")
	}
}

func TestRedactedDatabaseURL(t *testing.T) {
	got := RedactedDatabaseURL("postgres://user:pass@localhost:5432/cets")
	want := "postgres://***:***@localhost:5432/cets"
	if got != want {
		t.Fatalf("redacted URL = %q, want %q", got, want)
	}
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
