package ticketing

import (
	"log/slog"
	"time"

	"event-ticket-system/internal/ratelimit"
	"event-ticket-system/internal/reservation"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db                        *pgxpool.Pool
	signer                    Signer
	logger                    *slog.Logger
	now                       func() time.Time
	noShowPolicy              NoShowPolicy
	reservationGate           reservation.Gate
	reservationSecret         []byte
	bookingLimiter            ratelimit.Limiter
	rateLimitSecret           []byte
	bookingContentionStrategy string
}

func NewService(db *pgxpool.Pool, signer Signer, logger *slog.Logger) *Service {
	return NewServiceWithPolicy(db, signer, logger, DefaultNoShowPolicy())
}

func NewServiceWithPolicy(db *pgxpool.Pool, signer Signer, logger *slog.Logger, policy NoShowPolicy) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	policy = policy.Normalize()
	return &Service{
		db:                        db,
		signer:                    signer,
		logger:                    logger,
		now:                       func() time.Time { return time.Now().UTC() },
		noShowPolicy:              policy,
		reservationGate:           reservation.NoopGate{},
		bookingLimiter:            ratelimit.NoopLimiter{},
		bookingContentionStrategy: BookingContentionStrategyPhase1,
	}
}

// WithReservationGate attaches a PH2-22 pre-admission gate to the service.
// secret is the HMAC key used to derive Redis-safe idempotency hashes; it must
// be non-empty when gate.Enabled() is true. Calling with reservation.NoopGate
// (or not calling at all) preserves the Phase 1 DB-only booking path.
func (s *Service) WithReservationGate(gate reservation.Gate, secret []byte) *Service {
	if gate == nil {
		gate = reservation.NoopGate{}
	}
	s.reservationGate = gate
	s.reservationSecret = secret
	return s
}

// WithBookingRateLimiter attaches the PH2-21 booking admission limiter.
// secret hashes actor identifiers before they enter Redis, logs, or audit
// metadata. A nil limiter preserves the disabled path.
func (s *Service) WithBookingRateLimiter(limiter ratelimit.Limiter, secret []byte) *Service {
	if limiter == nil {
		limiter = ratelimit.NoopLimiter{}
	}
	s.bookingLimiter = limiter
	s.rateLimitSecret = secret
	return s
}

// WithBookingContentionStrategy selects how limited FCFS bookings queue before
// the final PostgreSQL capacity check. Invalid values preserve phase1 behavior.
func (s *Service) WithBookingContentionStrategy(strategy string) *Service {
	parsed, err := ParseBookingContentionStrategy(strategy)
	if err != nil {
		parsed = BookingContentionStrategyPhase1
	}
	s.bookingContentionStrategy = parsed
	return s
}

func (s *Service) WithClock(now func() time.Time) *Service {
	if now == nil {
		return s
	}
	s.now = func() time.Time { return now().UTC() }
	return s
}
