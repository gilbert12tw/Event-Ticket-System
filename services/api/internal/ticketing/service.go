package ticketing

import (
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db           *pgxpool.Pool
	signer       Signer
	logger       *slog.Logger
	now          func() time.Time
	noShowPolicy NoShowPolicy
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
		db:           db,
		signer:       signer,
		logger:       logger,
		now:          func() time.Time { return time.Now().UTC() },
		noShowPolicy: policy,
	}
}
