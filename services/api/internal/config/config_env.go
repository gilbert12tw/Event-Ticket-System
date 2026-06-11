package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

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

// parseExportsSourceEnv selects the report export data source (PH2-45).
// projection (default) reads reporting_event_summary; operational restores
// the Phase 1 OLTP aggregation as the rollback path.
func parseExportsSourceEnv(loadErrors *[]string) string {
	value := strings.ToLower(getEnv("EXPORTS_SOURCE", "projection"))
	switch value {
	case "projection", "operational":
		return value
	default:
		*loadErrors = append(*loadErrors, fmt.Sprintf("EXPORTS_SOURCE must be one of projection|operational, got %q", value))
		return "projection"
	}
}

// parseExportsStalePolicyEnv validates the stale-projection policy (PH2-45).
// Only fail (fail closed) is implemented; the enum exists so future policy
// values are explicit config features and typos fail Load() loudly.
func parseExportsStalePolicyEnv(loadErrors *[]string) string {
	value := strings.ToLower(getEnv("EXPORTS_STALE_POLICY", "fail"))
	if value == "fail" {
		return value
	}
	*loadErrors = append(*loadErrors, fmt.Sprintf("EXPORTS_STALE_POLICY must be fail, got %q", value))
	return "fail"
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
