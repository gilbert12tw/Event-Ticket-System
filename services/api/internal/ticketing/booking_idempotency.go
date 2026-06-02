package ticketing

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type bookingIdempotencyResult struct {
	IdempotencyKey     string
	EventID            string
	EmployeeID         string
	FamilyCount        int
	IdempotencyHash    string
	RegistrationID     string
	RegistrationStatus string
	TicketID           string
	RemainingCapacity  int
	Message            string
	CompletedAt        time.Time
}

func (s *Service) lockBookingIdempotencyResultTx(
	ctx context.Context,
	tx pgx.Tx,
	key string,
	eventID string,
	employeeID string,
	familyCount int,
	idempotencyHash string,
) (bookingIdempotencyResult, bool, error) {
	hash := sql.NullString{}
	if idempotencyHash != "" {
		hash = sql.NullString{String: idempotencyHash, Valid: true}
	}
	tag, err := tx.Exec(ctx, `INSERT INTO booking_idempotency_results
			(idempotency_key, event_id, employee_id, family_count, idempotency_hash)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		key, eventID, employeeID, familyCount, hash)
	if err != nil {
		return bookingIdempotencyResult{}, false, err
	}
	if tag.RowsAffected() == 1 {
		return bookingIdempotencyResult{}, false, nil
	}

	result, err := s.bookingIdempotencyResultForUpdateTx(ctx, tx, key)
	if err != nil {
		return bookingIdempotencyResult{}, false, err
	}
	if err := validateBookingIdempotencyResult(result, eventID, employeeID, familyCount); err != nil {
		return bookingIdempotencyResult{}, false, err
	}
	return result, true, nil
}

func validateBookingIdempotencyResult(result bookingIdempotencyResult, eventID string, employeeID string, familyCount int) error {
	if result.EventID != eventID || result.EmployeeID != employeeID || result.FamilyCount != familyCount {
		return conflict("idempotency key belongs to a different booking request")
	}
	if result.RegistrationID == "" || result.CompletedAt.IsZero() {
		return conflict("booking idempotency result is not ready")
	}
	return nil
}

func (s *Service) bookingIdempotencyResultForUpdateTx(ctx context.Context, tx pgx.Tx, key string) (bookingIdempotencyResult, error) {
	return scanBookingIdempotencyResult(tx.QueryRow(ctx, `SELECT idempotency_key, event_id, employee_id, family_count, idempotency_hash,
			registration_id, registration_status, ticket_id, remaining_capacity, message, completed_at
		FROM booking_idempotency_results
		WHERE idempotency_key = $1
		FOR UPDATE`, key))
}

func scanBookingIdempotencyResult(row pgx.Row) (bookingIdempotencyResult, error) {
	var result bookingIdempotencyResult
	var registrationID sql.NullString
	var ticketID sql.NullString
	var idempotencyHash sql.NullString
	var completedAt sql.NullTime
	err := row.Scan(&result.IdempotencyKey, &result.EventID, &result.EmployeeID, &result.FamilyCount, &idempotencyHash,
		&registrationID, &result.RegistrationStatus, &ticketID, &result.RemainingCapacity, &result.Message, &completedAt)
	if err != nil {
		return bookingIdempotencyResult{}, err
	}
	if registrationID.Valid {
		result.RegistrationID = registrationID.String
	}
	if ticketID.Valid {
		result.TicketID = ticketID.String
	}
	if idempotencyHash.Valid {
		result.IdempotencyHash = idempotencyHash.String
	}
	if completedAt.Valid {
		result.CompletedAt = completedAt.Time
	}
	return result, nil
}

func (s *Service) completeBookingIdempotencyResultTx(ctx context.Context, tx pgx.Tx, key string, response BookingResponse) error {
	ticketID := sql.NullString{}
	if response.Ticket != nil {
		ticketID = sql.NullString{String: response.Ticket.TicketID, Valid: true}
	}
	_, err := tx.Exec(ctx, `UPDATE booking_idempotency_results
		SET registration_id = $2,
			registration_status = $3,
			ticket_id = $4,
			remaining_capacity = $5,
			message = $6,
			completed_at = now()
		WHERE idempotency_key = $1`,
		key,
		response.Registration.RegistrationID,
		response.Registration.Status,
		ticketID,
		response.RemainingCapacity,
		response.Message)
	return err
}

func (s *Service) bookingResponseFromIdempotencyResultTx(ctx context.Context, tx pgx.Tx, result bookingIdempotencyResult) (BookingResponse, error) {
	reg, err := s.findRegistrationByIDTx(ctx, tx, result.RegistrationID)
	if err != nil {
		return BookingResponse{}, err
	}
	reg.Status = result.RegistrationStatus
	reg.IdempotencyKey = result.IdempotencyKey
	reg.FamilyCount = result.FamilyCount

	var ticket *Ticket
	if result.TicketID != "" {
		ticket, err = s.findTicketByRegistrationTx(ctx, tx, reg.RegistrationID)
		if err != nil {
			return BookingResponse{}, err
		}
		if ticket == nil || ticket.TicketID != result.TicketID {
			return BookingResponse{}, errors.New("booking idempotency ticket result is missing")
		}
	}
	return BookingResponse{
		Registration:      reg,
		Ticket:            ticket,
		RemainingCapacity: result.RemainingCapacity,
		Message:           result.Message,
		Duplicate:         true,
	}, nil
}
