package ticketing

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}

func insertAudit(ctx context.Context, tx pgx.Tx, auditID string, actor Actor, action string, entityType string, entityID string, metadata map[string]interface{}) error {
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs (audit_id, actor_id, role, action, entity_type, entity_id, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`, auditID, actor.ID, actor.Role, action, entityType, entityID, string(payload))
	return err
}

func insertOutbox(ctx context.Context, tx pgx.Tx, eventType string, aggregateID string, payload map[string]interface{}) error {
	outboxID, err := newID("out")
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events (outbox_id, aggregate_id, event_type, payload)
		VALUES ($1,$2,$3,$4::jsonb)`, outboxID, aggregateID, eventType, string(body))
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
