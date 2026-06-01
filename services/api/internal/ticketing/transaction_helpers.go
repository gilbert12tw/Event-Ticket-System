package ticketing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func rollback(ctx context.Context, tx pgx.Tx) {
	_ = tx.Rollback(ctx)
}

type auditRecord struct {
	auditID    string
	actor      Actor
	action     string
	entityType string
	entityID   string
	metadata   map[string]interface{}
}

func newAuditRecord(auditID string, actor Actor, action string, entityType string, entityID string, metadata map[string]interface{}) auditRecord {
	return auditRecord{auditID: auditID, actor: actor, action: action, entityType: entityType, entityID: entityID, metadata: metadata}
}

func insertAudit(ctx context.Context, tx pgx.Tx, audit auditRecord) error {
	return insertAuditWithExecutor(ctx, tx, audit)
}

type auditExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
}

func insertAuditWithExecutor(ctx context.Context, exec auditExecutor, audit auditRecord) error {
	payload, err := json.Marshal(audit.metadata)
	if err != nil {
		return err
	}
	_, err = exec.Exec(ctx, `INSERT INTO audit_logs (audit_id, actor_id, role, action, entity_type, entity_id, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`,
		audit.auditID, audit.actor.ID, audit.actor.Role, audit.action, audit.entityType, audit.entityID, string(payload))
	return err
}

func insertOutbox(ctx context.Context, tx pgx.Tx, eventType string, aggregateID string, payload map[string]interface{}) error {
	outboxID, err := newID("out")
	if err != nil {
		return err
	}
	idempotencyKey, partitionKey, err := outboxV2Metadata(eventType, aggregateID, outboxID, payload)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]interface{}{
		"event_id":        outboxID,
		"event_type":      eventType,
		"schema_version":  2,
		"occurred_at":     time.Now().UTC().Format(time.RFC3339Nano),
		"idempotency_key": idempotencyKey,
		"partition_key":   partitionKey,
		"payload":         payload,
	})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events
		(outbox_id, aggregate_id, event_type, payload, schema_version, idempotency_key, partition_key)
		VALUES ($1,$2,$3,$4::jsonb,2,$5,$6)`, outboxID, aggregateID, eventType, string(body), idempotencyKey, partitionKey)
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
