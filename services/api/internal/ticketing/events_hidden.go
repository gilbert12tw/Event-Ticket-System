package ticketing

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// loadHiddenEvents returns the set of event IDs the employee has hidden from their calendar view.
func (s *Service) loadHiddenEvents(ctx context.Context, employeeID string) (map[string]bool, error) {
	hidden := map[string]bool{}
	rows, err := s.db.Query(ctx, `SELECT event_id FROM hidden_events WHERE employee_id = $1`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			return nil, err
		}
		hidden[eventID] = true
	}
	return hidden, rows.Err()
}

// HideEvent hides an event from the calling employee's calendar view. This is a personal view
// preference, not booking state: the event still appears in the employee's event list flagged as
// hidden. The composite primary key makes a repeat hide a no-op, so the operation is idempotent.
func (s *Service) HideEvent(ctx context.Context, actor Actor, eventID string) error {
	if err := requireRole(actor, RoleEmployee); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO hidden_events (employee_id, event_id, hidden_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (employee_id, event_id) DO NOTHING`,
		actor.ID, eventID, s.now())
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		// Foreign key violation: the event (or employee) does not exist.
		return notFound("event not found")
	}
	return err
}

// UnhideEvent restores a previously hidden event to the calling employee's calendar view.
// Unhiding an event that is not currently hidden is a no-op, so the operation is idempotent.
func (s *Service) UnhideEvent(ctx context.Context, actor Actor, eventID string) error {
	if err := requireRole(actor, RoleEmployee); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `DELETE FROM hidden_events WHERE employee_id = $1 AND event_id = $2`,
		actor.ID, eventID)
	return err
}
