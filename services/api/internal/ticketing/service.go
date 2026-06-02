package ticketing

import (
	"log/slog"
	"time"

	"event-ticket-system/internal/observability"
	"event-ticket-system/internal/reservation"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db                *pgxpool.Pool
	signer            Signer
	logger            *slog.Logger
	now               func() time.Time
	noShowPolicy      NoShowPolicy
	reservationGate   reservation.Gate
	reservationSecret []byte
	metrics           *observability.Registry
	reservationOutage string
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
		db:              db,
		signer:          signer,
		logger:          logger,
		now:             func() time.Time { return time.Now().UTC() },
		noShowPolicy:    policy,
		reservationGate: reservation.NoopGate{},
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

func (s *Service) WithMetrics(metrics *observability.Registry) *Service {
	s.metrics = metrics
	return s
}

func (s *Service) WithReservationOutageMode(outageMode string) *Service {
	s.reservationOutage = outageMode
	return s
}

func (s *Service) WithClock(now func() time.Time) *Service {
	if now == nil {
		return s
	}
	s.now = func() time.Time { return now().UTC() }
	return s
}
