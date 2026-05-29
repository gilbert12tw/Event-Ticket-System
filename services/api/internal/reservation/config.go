package reservation

import (
	"errors"
	"strings"
	"time"
)

// OutageMode controls the gate's behavior when Redis is unreachable.
type OutageMode string

const (
	// OutageModeDegrade falls back to DB-only admission (fail open). The
	// existing PostgreSQL transaction still prevents oversell.
	OutageModeDegrade OutageMode = "degrade"
	// OutageModeFail returns a controlled error so the API layer can emit
	// a 503 with retry metadata (fail closed).
	OutageModeFail OutageMode = "fail"
)

// Config carries the operational settings for the Redis gate. All values come
// from environment variables; the booking application service does not own
// any process-local reservation state.
type Config struct {
	Enabled          bool
	OutageMode       OutageMode
	HashSecret       []byte
	TTL              time.Duration
	GraceTTL         time.Duration
	OperationTimeout time.Duration
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.HashSecret) == 0 {
		return errors.New("BOOKING_RESERVATION_HASH_SECRET is required when BOOKING_PREADMISSION=on")
	}
	if c.TTL <= 0 {
		return errors.New("RESERVATION_TTL_SECONDS must be positive when BOOKING_PREADMISSION=on")
	}
	if c.OperationTimeout <= 0 {
		return errors.New("REDIS_OPERATION_TIMEOUT_MS must be positive when BOOKING_PREADMISSION=on")
	}
	switch c.OutageMode {
	case OutageModeDegrade, OutageModeFail:
	default:
		return errors.New("REDIS_OUTAGE_MODE must be one of degrade|fail")
	}
	return nil
}

// ParseOutageMode normalizes an env-supplied outage mode string. Empty input
// returns the safe default (degrade).
func ParseOutageMode(value string) (OutageMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(OutageModeDegrade):
		return OutageModeDegrade, nil
	case string(OutageModeFail):
		return OutageModeFail, nil
	default:
		return "", errors.New("REDIS_OUTAGE_MODE must be one of degrade|fail")
	}
}
