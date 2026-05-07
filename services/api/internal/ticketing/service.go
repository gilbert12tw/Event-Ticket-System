package ticketing

import (
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db     *pgxpool.Pool
	signer Signer
	logger *slog.Logger
	now    func() time.Time
}

func NewService(db *pgxpool.Pool, signer Signer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		db:     db,
		signer: signer,
		logger: logger,
		now:    func() time.Time { return time.Now().UTC() },
	}
}
