package ticketing

import (
	"context"
	"fmt"
	"strings"
)

func (s *Service) AuditLogs(ctx context.Context, actor Actor, query ...AuditLogQuery) ([]AuditLog, error) {
	if err := requireRole(actor, RoleHRAdmin); err != nil {
		return nil, err
	}
	q := effectiveAuditQuery(query)
	where := []string{"true"}
	args := []interface{}{}
	appendAuditFilter(&where, &args, "actor_id", q.ActorID)
	appendAuditFilter(&where, &args, "role", q.Role)
	appendAuditFilter(&where, &args, "action", q.Action)
	appendAuditFilter(&where, &args, "entity_type", q.EntityType)
	appendAuditFilter(&where, &args, "entity_id", q.EntityID)
	appendAuditTimeFilter(&where, &args, "created_at", ">=", q.From)
	appendAuditTimeFilter(&where, &args, "created_at", "<=", q.To)
	appendAuditCursorFilter(&where, &args, q.Cursor, q.CursorID)
	args = append(args, parseAuditLimit(q.Limit))
	sql := fmt.Sprintf(`SELECT audit_id, actor_id, role, action, entity_type, entity_id, metadata::text, created_at
		FROM audit_logs
		WHERE %s
		ORDER BY created_at DESC, audit_id DESC
		LIMIT $%d`, strings.Join(where, " AND "), len(args))
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var log AuditLog
		if err := rows.Scan(&log.AuditID, &log.ActorID, &log.Role, &log.Action, &log.EntityType, &log.EntityID, &log.Metadata, &log.CreatedAt); err != nil {
			return nil, err
		}
		log.Metadata = redactAuditMetadata(log.Metadata)
		logs = append(logs, log)
	}
	return logs, rows.Err()
}
