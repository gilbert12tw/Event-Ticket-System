package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	localTokenSecret    = "local-dev-token-secret"
	localProviderSecret = "local-dev-provider-token-secret"
	demoTokenSecret     = "local_dev_ticket_signing_secret_change_me"
	demoProviderSecret  = "local_dev_provider_token_secret_change_me"
	authModeLocalSSO    = "local_sso"
	authModeExternalSSO = "external_sso"
	authModeOIDC        = "oidc"
	authModeSAML        = "saml"

	minProductionSecretLength = 32
)

type Config struct {
	AppAddr                         string
	AppEnv                          string
	AuthMode                        string
	DatabaseURL                     string
	DatabaseReadURL                 string
	DatabaseWriteURL                string
	RedisURL                        string
	QueueURL                        string
	ObjectEndpoint                  string
	ObjectBucket                    string
	ObjectRegion                    string
	ObjectAccessKey                 string
	ObjectSecretKey                 string
	MailerHost                      string
	MailerPort                      int
	MailerFrom                      string
	MailerRedirectTo                string
	TokenSigningSecret              string
	ProviderTokenSecret             string
	AutoMigrate                     bool
	OpsAPIEnabled                   bool
	DemoDebugEnabled                bool
	RequestTimeout                  time.Duration
	DatabaseTimeout                 time.Duration
	ShutdownTimeout                 time.Duration
	WorkerPollInterval              time.Duration
	WorkerShutdownGrace             time.Duration
	WorkerMaxAttempts               int
	WorkerBatchSize                 int
	OutboxLeaseTTL                  time.Duration
	OutboxRetryMax                  int
	OutboxBackoffBase               time.Duration
	OutboxBackoffMax                time.Duration
	WorkerKinds                     []string
	WorkerConcurrency               map[string]int
	NoShowThreshold                 int
	NoShowCooldownDays              int
	NoShowGraceHours                int
	ReportStaleThresholdSeconds     int
	ReportUnavailableTimeoutSeconds int
	RebuildDryRun                   bool
	RebuildSampleValidate           bool
	RateLimitEnabled                bool
	BookingRateLimitPerActor        int
	BookingRateLimitPerEvent        int
	RateLimitOutageMode             string
	BookingRateLimitHashSecret      string
	BookingPreadmission             bool
	BookingContentionStrategy       string
	ReservationOutageMode           string
	ReservationTTL                  time.Duration
	ReservationGraceTTL             time.Duration
	ReservationOperationTimeout     time.Duration
	ReservationCompensationInterval time.Duration
	BookingReservationHashSecret    string
	OTelTracesEnabled               bool
	OTelEndpoint                    string
	OTelServiceName                 string
	OTelServiceVersion              string
	PyroscopeEnabled                bool
	PyroscopeAddress                string
	PyroscopeAppName                string
	loadErrors                      []string
}

func Load() Config {
	var loadErrors []string
	otelServiceName := getEnv("OTEL_SERVICE_NAME", "cets-api")
	return Config{
		AppAddr:                         getEnv("APP_ADDR", ":8080"),
		AppEnv:                          getEnv("APP_ENV", "local"),
		AuthMode:                        getEnv("AUTH_MODE", authModeExternalSSO),
		DatabaseURL:                     os.Getenv("DATABASE_URL"),
		DatabaseReadURL:                 strings.TrimSpace(os.Getenv("DATABASE_READ_URL")),
		DatabaseWriteURL:                strings.TrimSpace(os.Getenv("DATABASE_WRITE_URL")),
		RedisURL:                        os.Getenv("REDIS_URL"),
		QueueURL:                        os.Getenv("QUEUE_URL"),
		ObjectEndpoint:                  os.Getenv("OBJECT_STORAGE_ENDPOINT"),
		ObjectBucket:                    os.Getenv("OBJECT_STORAGE_BUCKET"),
		ObjectRegion:                    getEnv("OBJECT_STORAGE_REGION", "us-east-1"),
		ObjectAccessKey:                 os.Getenv("OBJECT_STORAGE_ACCESS_KEY"),
		ObjectSecretKey:                 os.Getenv("OBJECT_STORAGE_SECRET_KEY"),
		MailerHost:                      getEnv("MAILER_HOST", "localhost"),
		MailerPort:                      parsePositiveIntEnv("MAILER_PORT", "1025", &loadErrors),
		MailerFrom:                      getEnv("MAILER_FROM", "no-reply@cets.local"),
		MailerRedirectTo:                strings.TrimSpace(os.Getenv("MAILER_REDIRECT_TO")),
		TokenSigningSecret:              getEnv("TOKEN_SIGNING_SECRET", localTokenSecret),
		ProviderTokenSecret:             getEnv("PROVIDER_TOKEN_SECRET", localProviderSecret),
		AutoMigrate:                     parseBoolEnv("AUTO_MIGRATE", "false", &loadErrors),
		OpsAPIEnabled:                   parseBoolEnv("OPS_API_ENABLED", "false", &loadErrors),
		DemoDebugEnabled:                parseBoolEnv("DEMO_DEBUG_ENABLED", "false", &loadErrors),
		RequestTimeout:                  parseDurationMSEnv("REQUEST_TIMEOUT_MS", "5000", &loadErrors),
		DatabaseTimeout:                 parseDurationMSEnv("DATABASE_TIMEOUT_MS", "5000", &loadErrors),
		ShutdownTimeout:                 parseDurationMSEnv("SHUTDOWN_TIMEOUT_MS", "10000", &loadErrors),
		WorkerPollInterval:              parseDurationMSEnv("WORKER_POLL_INTERVAL_MS", "1000", &loadErrors),
		WorkerShutdownGrace:             parseNonNegativeDurationSecondsEnv("WORKER_SHUTDOWN_GRACE_SECONDS", "30", &loadErrors),
		WorkerMaxAttempts:               parsePositiveIntEnv("WORKER_MAX_ATTEMPTS", "3", &loadErrors),
		WorkerBatchSize:                 parsePositiveIntEnvAlias("OUTBOX_BATCH_SIZE", "WORKER_BATCH_SIZE", "100", &loadErrors),
		OutboxLeaseTTL:                  parsePositiveDurationSecondsEnv("OUTBOX_LEASE_TTL_SECONDS", "60", &loadErrors),
		OutboxRetryMax:                  parseNonNegativeIntEnv("OUTBOX_RETRY_MAX", "10", &loadErrors),
		OutboxBackoffBase:               parseDurationMSEnv("OUTBOX_BACKOFF_BASE_MS", "500", &loadErrors),
		OutboxBackoffMax:                parseDurationMSEnv("OUTBOX_BACKOFF_MAX_MS", "60000", &loadErrors),
		WorkerKinds:                     parseWorkerKindsEnv(&loadErrors),
		WorkerConcurrency:               parseWorkerConcurrencyEnv(&loadErrors),
		NoShowThreshold:                 parsePositiveIntEnv("NO_SHOW_THRESHOLD", "1", &loadErrors),
		NoShowCooldownDays:              parsePositiveIntEnv("NO_SHOW_COOLDOWN_DAYS", "90", &loadErrors),
		NoShowGraceHours:                parsePositiveIntEnv("NO_SHOW_GRACE_HOURS", "24", &loadErrors),
		ReportStaleThresholdSeconds:     parsePositiveIntEnv("REPORT_STALE_THRESHOLD_SECONDS", "60", &loadErrors),
		ReportUnavailableTimeoutSeconds: parsePositiveIntEnv("REPORT_UNAVAILABLE_TIMEOUT_SECONDS", "180", &loadErrors),
		RebuildDryRun:                   parseBoolEnv("REBUILD_DRY_RUN", "false", &loadErrors),
		RebuildSampleValidate:           parseBoolEnv("REBUILD_SAMPLE_VALIDATE", "true", &loadErrors),
		RateLimitEnabled:                parseBoolEnv("RATE_LIMIT_ENABLED", "false", &loadErrors),
		BookingRateLimitPerActor:        parseNonNegativeIntEnv("BOOKING_RATE_LIMIT_RPS_PER_ACTOR", "0", &loadErrors),
		BookingRateLimitPerEvent:        parseNonNegativeIntEnv("BOOKING_RATE_LIMIT_RPS_PER_EVENT", "0", &loadErrors),
		RateLimitOutageMode:             getEnv("RATE_LIMIT_OUTAGE_MODE", "degrade"),
		BookingRateLimitHashSecret:      os.Getenv("BOOKING_RATE_LIMIT_HASH_SECRET"),
		BookingPreadmission:             parseOnOffEnv("BOOKING_PREADMISSION", "off", &loadErrors),
		BookingContentionStrategy:       parseBookingContentionStrategyEnv(&loadErrors),
		ReservationOutageMode:           getEnv("REDIS_OUTAGE_MODE", "degrade"),
		ReservationTTL:                  parseSecondsEnv("RESERVATION_TTL_SECONDS", "20", &loadErrors),
		ReservationGraceTTL:             parseSecondsEnv("RESERVATION_TTL_GRACE_SECONDS", "10", &loadErrors),
		ReservationOperationTimeout:     parseDurationMSEnv("REDIS_OPERATION_TIMEOUT_MS", "150", &loadErrors),
		ReservationCompensationInterval: parseSecondsEnv("RESERVATION_COMPENSATION_INTERVAL_SECONDS", "30", &loadErrors),
		BookingReservationHashSecret:    os.Getenv("BOOKING_RESERVATION_HASH_SECRET"),
		OTelTracesEnabled:               parseBoolEnv("OTEL_TRACES_ENABLED", "false", &loadErrors),
		OTelEndpoint:                    strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
		OTelServiceName:                 otelServiceName,
		OTelServiceVersion:              getEnv("OTEL_SERVICE_VERSION", "dev"),
		PyroscopeEnabled:                parseBoolEnv("PYROSCOPE_ENABLED", "false", &loadErrors),
		PyroscopeAddress:                strings.TrimSpace(os.Getenv("PYROSCOPE_SERVER_ADDRESS")),
		PyroscopeAppName:                getEnv("PYROSCOPE_APPLICATION_NAME", otelServiceName),
		loadErrors:                      loadErrors,
	}
}

func (c Config) ValidateForServe() error {
	if err := c.ValidateDatabase(); err != nil {
		return err
	}
	if strings.TrimSpace(c.AppAddr) == "" {
		return errors.New("APP_ADDR is required")
	}
	if c.RequestTimeout <= 0 {
		return errors.New("REQUEST_TIMEOUT_MS must be positive")
	}
	if c.ShutdownTimeout <= 0 {
		return errors.New("SHUTDOWN_TIMEOUT_MS must be positive")
	}
	if err := c.validateDemoDebug(); err != nil {
		return err
	}
	if err := c.validateRuntimeObservability(); err != nil {
		return err
	}
	if c.isProduction() {
		if err := c.validateProductionAuth(); err != nil {
			return err
		}
		if err := c.validateProductionBackingServices(); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) ValidateDatabase() error {
	if err := c.validateLoadedConfig(); err != nil {
		return err
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	if c.DatabaseTimeout <= 0 {
		return errors.New("DATABASE_TIMEOUT_MS must be positive")
	}
	return nil
}

func (c Config) validateLoadedConfig() error {
	if len(c.loadErrors) == 0 {
		return nil
	}
	return fmt.Errorf("invalid configuration: %s", strings.Join(c.loadErrors, "; "))
}

func (c Config) ValidateWorker() error {
	if err := c.ValidateDatabase(); err != nil {
		return err
	}
	if err := c.validateWorkerTiming(); err != nil {
		return err
	}
	if err := c.validateWorkerDispatch(); err != nil {
		return err
	}
	if err := c.validateWorkerMailer(); err != nil {
		return err
	}
	if err := c.validateDemoDebug(); err != nil {
		return err
	}
	if err := c.validateRuntimeObservability(); err != nil {
		return err
	}
	return c.validateProductionWorker()
}

func (c Config) validateWorkerTiming() error {
	if c.WorkerPollInterval <= 0 {
		return errors.New("WORKER_POLL_INTERVAL_MS must be positive")
	}
	if c.WorkerShutdownGrace < 0 {
		return errors.New("WORKER_SHUTDOWN_GRACE_SECONDS must be non-negative")
	}
	if c.OutboxLeaseTTL <= 0 {
		return errors.New("OUTBOX_LEASE_TTL_SECONDS must be positive")
	}
	if c.OutboxBackoffBase <= 0 {
		return errors.New("OUTBOX_BACKOFF_BASE_MS must be positive")
	}
	if c.OutboxBackoffMax <= 0 {
		return errors.New("OUTBOX_BACKOFF_MAX_MS must be positive")
	}
	if c.OutboxBackoffMax < c.OutboxBackoffBase {
		return errors.New("OUTBOX_BACKOFF_MAX_MS must be greater than or equal to OUTBOX_BACKOFF_BASE_MS")
	}
	return nil
}

func (c Config) validateWorkerDispatch() error {
	if c.WorkerMaxAttempts <= 0 {
		return errors.New("WORKER_MAX_ATTEMPTS must be positive")
	}
	if c.WorkerBatchSize <= 0 {
		return errors.New("OUTBOX_BATCH_SIZE must be positive")
	}
	if c.OutboxRetryMax < 0 {
		return errors.New("OUTBOX_RETRY_MAX must be non-negative")
	}
	if err := validateWorkerKinds(c.WorkerKinds); err != nil {
		return err
	}
	return validateWorkerConcurrency(c.WorkerConcurrency)
}

func (c Config) validateWorkerMailer() error {
	if strings.TrimSpace(c.MailerHost) == "" {
		return errors.New("MAILER_HOST is required")
	}
	if c.MailerPort <= 0 {
		return errors.New("MAILER_PORT must be positive")
	}
	if strings.TrimSpace(c.MailerFrom) == "" {
		return errors.New("MAILER_FROM is required")
	}
	return nil
}

func (c Config) validateProductionWorker() error {
	if !c.isProduction() {
		return nil
	}
	if err := validateProductionSecret("TOKEN_SIGNING_SECRET", c.TokenSigningSecret, localTokenSecret, demoTokenSecret); err != nil {
		return err
	}
	return c.validateProductionBackingServices()
}

func (c Config) isProduction() bool {
	return strings.EqualFold(strings.TrimSpace(c.AppEnv), "production")
}

func (c Config) validateDemoDebug() error {
	if !c.DemoDebugEnabled {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(c.AppEnv)) {
	case "local", "demo", "test":
		return nil
	default:
		return errors.New("DEMO_DEBUG_ENABLED is only allowed when APP_ENV is local, demo, or test")
	}
}

func (c Config) validateProductionAuth() error {
	switch strings.ToLower(strings.TrimSpace(c.AuthMode)) {
	case "":
		return errors.New("AUTH_MODE is required in production")
	case authModeLocalSSO, "local", "demo":
		return errors.New("AUTH_MODE must not use local/demo authentication in production")
	case authModeExternalSSO, authModeOIDC, authModeSAML:
	default:
		return fmt.Errorf("AUTH_MODE %q is not supported in production", c.AuthMode)
	}
	if err := validateProductionSecret("TOKEN_SIGNING_SECRET", c.TokenSigningSecret, localTokenSecret, demoTokenSecret); err != nil {
		return err
	}
	if err := validateProductionSecret("PROVIDER_TOKEN_SECRET", c.ProviderTokenSecret, localProviderSecret, demoProviderSecret); err != nil {
		return err
	}
	return nil
}

func validateProductionSecret(name string, value string, unsafeValues ...string) error {
	secret := strings.TrimSpace(value)
	if secret == "" {
		return fmt.Errorf("%s must be set in production", name)
	}
	for _, unsafe := range unsafeValues {
		if secret == unsafe {
			return fmt.Errorf("%s must not use local/demo value in production", name)
		}
	}
	if len(secret) < minProductionSecretLength {
		return fmt.Errorf("%s must be at least %d characters in production", name, minProductionSecretLength)
	}
	return nil
}

func (c Config) validateProductionBackingServices() error {
	required := []struct {
		name  string
		value string
	}{
		{"REDIS_URL", c.RedisURL},
		{"QUEUE_URL", c.QueueURL},
		{"OBJECT_STORAGE_ENDPOINT", c.ObjectEndpoint},
		{"OBJECT_STORAGE_BUCKET", c.ObjectBucket},
		{"OBJECT_STORAGE_ACCESS_KEY", c.ObjectAccessKey},
		{"OBJECT_STORAGE_SECRET_KEY", c.ObjectSecretKey},
		{"MAILER_HOST", c.MailerHost},
		{"MAILER_FROM", c.MailerFrom},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s is required in production", field.name)
		}
	}
	if c.MailerPort <= 0 {
		return errors.New("MAILER_PORT must be positive")
	}
	return nil
}

func getEnv(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func parseBoolEnv(key string, fallback string, loadErrors *[]string) bool {
	value := getEnv(key, fallback)
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		*loadErrors = append(*loadErrors, fmt.Sprintf("%s must be a boolean, got %q", key, value))
		return false
	}
	return parsed
}

func parseDurationMSEnv(key string, fallback string, loadErrors *[]string) time.Duration {
	value := getEnv(key, fallback)
	ms, err := strconv.Atoi(value)
	if err != nil || ms <= 0 {
		*loadErrors = append(*loadErrors, fmt.Sprintf("%s must be a positive integer of milliseconds, got %q", key, value))
		return 5 * time.Second
	}
	return time.Duration(ms) * time.Millisecond
}

func parseNonNegativeDurationSecondsEnv(key string, fallback string, loadErrors *[]string) time.Duration {
	value := getEnv(key, fallback)
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		*loadErrors = append(*loadErrors, fmt.Sprintf("%s must be a non-negative integer of seconds, got %q", key, value))
		return 30 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func parsePositiveDurationSecondsEnv(key string, fallback string, loadErrors *[]string) time.Duration {
	value := getEnv(key, fallback)
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		*loadErrors = append(*loadErrors, fmt.Sprintf("%s must be a positive integer of seconds, got %q", key, value))
		fallbackValue, fallbackErr := strconv.Atoi(fallback)
		if fallbackErr != nil || fallbackValue <= 0 {
			return time.Second
		}
		return time.Duration(fallbackValue) * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func parseSecondsEnv(key string, fallback string, loadErrors *[]string) time.Duration {
	value := getEnv(key, fallback)
	secs, err := strconv.Atoi(value)
	if err != nil || secs <= 0 {
		*loadErrors = append(*loadErrors, fmt.Sprintf("%s must be a positive integer of seconds, got %q", key, value))
		return 0
	}
	return time.Duration(secs) * time.Second
}

func parseOnOffEnv(key string, fallback string, loadErrors *[]string) bool {
	value := strings.ToLower(getEnv(key, fallback))
	switch value {
	case "on", "true", "1", "yes":
		return true
	case "off", "false", "0", "no", "":
		return false
	default:
		*loadErrors = append(*loadErrors, fmt.Sprintf("%s must be one of on|off, got %q", key, value))
		return false
	}
}

func parseBookingContentionStrategyEnv(loadErrors *[]string) string {
	value := strings.ToLower(getEnv("BOOKING_CONTENTION_STRATEGY", "phase1"))
	switch value {
	case "phase1", "advisory":
		return value
	default:
		*loadErrors = append(*loadErrors, fmt.Sprintf("BOOKING_CONTENTION_STRATEGY must be one of phase1|advisory, got %q", value))
		return "phase1"
	}
}

func parsePositiveIntEnv(key string, fallback string, loadErrors *[]string) int {
	value := getEnv(key, fallback)
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		*loadErrors = append(*loadErrors, fmt.Sprintf("%s must be a positive integer, got %q", key, value))
		fallbackValue, fallbackErr := strconv.Atoi(fallback)
		if fallbackErr != nil || fallbackValue <= 0 {
			return 1
		}
		return fallbackValue
	}
	return parsed
}

func parsePositiveIntEnvAlias(primary string, legacy string, fallback string, loadErrors *[]string) int {
	if strings.TrimSpace(os.Getenv(primary)) != "" {
		return parsePositiveIntEnv(primary, fallback, loadErrors)
	}
	if strings.TrimSpace(os.Getenv(legacy)) != "" {
		return parsePositiveIntEnv(legacy, fallback, loadErrors)
	}
	return parsePositiveIntEnv(primary, fallback, loadErrors)
}

func parseNonNegativeIntEnv(key string, fallback string, loadErrors *[]string) int {
	value := getEnv(key, fallback)
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		*loadErrors = append(*loadErrors, fmt.Sprintf("%s must be a non-negative integer, got %q", key, value))
		fallbackValue, fallbackErr := strconv.Atoi(fallback)
		if fallbackErr != nil || fallbackValue < 0 {
			return 0
		}
		return fallbackValue
	}
	return parsed
}

func RedactedDatabaseURL(databaseURL string) string {
	if databaseURL == "" {
		return ""
	}
	at := strings.LastIndex(databaseURL, "@")
	schemeEnd := strings.Index(databaseURL, "://")
	if at == -1 || schemeEnd == -1 || schemeEnd > at {
		return databaseURL
	}
	return fmt.Sprintf("%s://***:***%s", databaseURL[:schemeEnd], databaseURL[at:])
}
