package ticketing

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func (s *Service) NotificationDeliveryOpsFeed(ctx context.Context, actor Actor, query ...NotificationDeliveryOpsQuery) (NotificationDeliveryOpsPage, error) {
	if err := s.requireOpsFeedRole(ctx, actor, "notification_deliveries"); err != nil {
		return NotificationDeliveryOpsPage{}, err
	}
	q, err := effectiveNotificationDeliveryOpsQuery(query)
	if err != nil {
		return NotificationDeliveryOpsPage{}, err
	}
	where := []string{"true"}
	args := []interface{}{}
	appendNotificationDeliveryStatusFilter(&where, &args, q.Status)
	appendNotificationDeliveryCursorFilter(&where, &args, q)
	args = append(args, q.Limit+1)
	sqlQuery := fmt.Sprintf(`SELECT nd.delivery_id, COALESCE(nd.outbox_id, ''), COALESCE(nd.employee_id, ''), nd.channel, nd.status,
			nd.last_error, nd.created_at, nd.updated_at, COALESCE(oe.event_type, ''), COALESCE(oe.retry_count, 0), oe.dead_letter_at
		FROM notification_deliveries nd
		LEFT JOIN outbox_events oe ON oe.outbox_id = nd.outbox_id
		WHERE %s
		ORDER BY nd.updated_at DESC, nd.delivery_id DESC
		LIMIT $%d`, strings.Join(where, " AND "), len(args))
	rows, err := s.db.Query(ctx, sqlQuery, args...)
	if err != nil {
		return NotificationDeliveryOpsPage{}, err
	}
	defer rows.Close()
	deliveries := make([]NotificationDeliveryOpsRow, 0, q.Limit)
	for rows.Next() {
		row, err := scanNotificationDeliveryOpsRow(rows)
		if err != nil {
			return NotificationDeliveryOpsPage{}, err
		}
		deliveries = append(deliveries, row)
	}
	if err := rows.Err(); err != nil {
		return NotificationDeliveryOpsPage{}, err
	}
	page := NotificationDeliveryOpsPage{Deliveries: deliveries}
	if len(deliveries) > q.Limit {
		page.Deliveries = deliveries[:q.Limit]
		last := page.Deliveries[len(page.Deliveries)-1]
		page.NextCursor = EncodeNotificationDeliveryCursor(*last.LastAttemptAt, last.DeliveryID)
	}
	return page, nil
}

type notificationDeliveryOpsScanner interface {
	Scan(dest ...interface{}) error
}

func scanNotificationDeliveryOpsRow(row notificationDeliveryOpsScanner) (NotificationDeliveryOpsRow, error) {
	var outboxID, employeeID, channel, eventType string
	var delivery NotificationDeliveryOpsRow
	var updatedAt sql.NullTime
	var deadLetterAt sql.NullTime
	err := row.Scan(&delivery.DeliveryID, &outboxID, &employeeID, &channel, &delivery.Status, &delivery.LastError,
		&delivery.CreatedAt, &updatedAt, &eventType, &delivery.RetryCount, &deadLetterAt)
	if err != nil {
		return NotificationDeliveryOpsRow{}, err
	}
	eventType = strings.TrimSpace(eventType)
	delivery.EventType = safeOutboxTelemetryEventType(eventType)
	delivery.WorkerKind = safeOutboxTelemetryWorkerKind(eventType)
	delivery.RecipientRedacted = notificationEmployeeRef(employeeID)
	delivery.LastError = redactNotificationDeliveryError(delivery.LastError, employeeID)
	if updatedAt.Valid {
		attemptedAt := updatedAt.Time
		delivery.LastAttemptAt = &attemptedAt
	}
	if deadLetterAt.Valid {
		at := deadLetterAt.Time
		delivery.DeadLetterAt = &at
	}
	delivery.RetryEligible = retryableNotificationOutboxEventType(eventType) &&
		channel == "email" &&
		outboxID != "" &&
		(delivery.Status == deliveryStatusFailed || delivery.Status == deliveryStatusDeadLetter)
	delivery.DeadLetterEligible = delivery.Status == deliveryStatusDeadLetter || delivery.DeadLetterAt != nil
	return delivery, nil
}

func appendNotificationDeliveryStatusFilter(parts *[]string, args *[]interface{}, status string) {
	status = strings.TrimSpace(status)
	if status == "" {
		return
	}
	*args = append(*args, status)
	*parts = append(*parts, fmt.Sprintf("nd.status = $%d", len(*args)))
}

func appendNotificationDeliveryCursorFilter(parts *[]string, args *[]interface{}, q NotificationDeliveryOpsQuery) {
	if q.Cursor.IsZero() {
		return
	}
	*args = append(*args, q.Cursor)
	cursorIndex := len(*args)
	*args = append(*args, strings.TrimSpace(q.CursorID))
	*parts = append(*parts, fmt.Sprintf("(nd.updated_at < $%d OR (nd.updated_at = $%d AND nd.delivery_id < $%d))", cursorIndex, cursorIndex, len(*args)))
}
