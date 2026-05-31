package ticketing

import (
	"context"
	"errors"
	"strings"

	"event-ticket-system/internal/traceid"

	"github.com/jackc/pgx/v5"
)

func (s *Service) GetNotificationPreferences(ctx context.Context, actor Actor) (NotificationPreferences, error) {
	if err := requireRole(actor, RoleEmployee); err != nil {
		return NotificationPreferences{}, err
	}
	prefs, err := s.findNotificationPreferences(ctx, actor.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return NotificationPreferences{EmployeeID: actor.ID, EmailEnabled: true, InAppEnabled: true}, nil
	}
	return prefs, err
}

func (s *Service) UpdateNotificationPreferences(ctx context.Context, actor Actor, req NotificationPreferences) (NotificationPreferences, error) {
	if err := requireRole(actor, RoleEmployee); err != nil {
		return NotificationPreferences{}, err
	}
	updatedAt := s.now()
	_, err := s.db.Exec(ctx, `INSERT INTO notification_preferences (employee_id, email_enabled, in_app_enabled, opted_out_categories, updated_at)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (employee_id) DO UPDATE SET
			email_enabled = EXCLUDED.email_enabled,
			in_app_enabled = EXCLUDED.in_app_enabled,
			opted_out_categories = EXCLUDED.opted_out_categories,
			updated_at = EXCLUDED.updated_at`,
		actor.ID, req.EmailEnabled, req.InAppEnabled, joinTags(req.OptedOutCategories), updatedAt)
	if err != nil {
		return NotificationPreferences{}, err
	}
	return NotificationPreferences{EmployeeID: actor.ID, EmailEnabled: req.EmailEnabled, InAppEnabled: req.InAppEnabled, OptedOutCategories: normalizeTags(req.OptedOutCategories), UpdatedAt: updatedAt}, nil
}

func (s *Service) NotificationDeliveries(ctx context.Context, actor Actor) ([]NotificationDelivery, error) {
	if err := requireAnyRole(actor, RoleActivityAdmin, RoleHRAdmin); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT delivery_id, COALESCE(outbox_id, ''), COALESCE(employee_id, ''), channel, status, attempts, last_error, created_at, updated_at
		FROM notification_deliveries
		ORDER BY updated_at DESC
		LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deliveries []NotificationDelivery
	for rows.Next() {
		var delivery NotificationDelivery
		var employeeID string
		if err := rows.Scan(&delivery.DeliveryID, &delivery.OutboxID, &employeeID, &delivery.Channel, &delivery.Status, &delivery.Attempts, &delivery.LastError, &delivery.CreatedAt, &delivery.UpdatedAt); err != nil {
			return nil, err
		}
		delivery.EmployeeRef = notificationEmployeeRef(employeeID)
		delivery.LastError = redactNotificationDeliveryError(delivery.LastError, employeeID)
		deliveries = append(deliveries, delivery)
	}
	return deliveries, rows.Err()
}

func (s *Service) RetryNotificationDelivery(ctx context.Context, actor Actor, deliveryID string) (NotificationDelivery, error) {
	if err := requireAnyRole(actor, RoleActivityAdmin, RoleHRAdmin); err != nil {
		return NotificationDelivery{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return NotificationDelivery{}, err
	}
	defer rollback(ctx, tx)
	var delivery NotificationDelivery
	var employeeID string
	err = tx.QueryRow(ctx, `SELECT delivery_id, COALESCE(outbox_id, ''), COALESCE(employee_id, ''), channel, status, attempts, last_error, created_at, updated_at
		FROM notification_deliveries
		WHERE delivery_id = $1
		FOR UPDATE`, deliveryID).
		Scan(&delivery.DeliveryID, &delivery.OutboxID, &employeeID, &delivery.Channel, &delivery.Status, &delivery.Attempts, &delivery.LastError, &delivery.CreatedAt, &delivery.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return NotificationDelivery{}, notFound("delivery not found")
	}
	if err != nil {
		return NotificationDelivery{}, err
	}
	if delivery.Channel != "email" {
		return NotificationDelivery{}, conflict("only email deliveries can be retried")
	}
	if delivery.Status != deliveryStatusFailed && delivery.Status != deliveryStatusDeadLetter {
		return NotificationDelivery{}, conflict("only failed notification deliveries can be retried")
	}
	if delivery.OutboxID == "" {
		return NotificationDelivery{}, conflict("notification delivery outbox is unavailable")
	}
	var outboxEventType string
	err = tx.QueryRow(ctx, `SELECT event_type FROM outbox_events WHERE outbox_id = $1 FOR UPDATE`, delivery.OutboxID).Scan(&outboxEventType)
	if errors.Is(err, pgx.ErrNoRows) {
		return NotificationDelivery{}, conflict("notification delivery outbox is unavailable")
	}
	if err != nil {
		return NotificationDelivery{}, err
	}
	if !retryableNotificationOutboxEventType(outboxEventType) {
		return NotificationDelivery{}, conflict("only notification outbox deliveries can be retried")
	}
	previousStatus := delivery.Status
	outboxID := delivery.OutboxID
	channel := delivery.Channel
	deadLetterCleared := previousStatus == deliveryStatusDeadLetter
	err = tx.QueryRow(ctx, `UPDATE notification_deliveries
		SET status = 'pending', last_error = '', updated_at = now()
		WHERE delivery_id = $1
		RETURNING delivery_id, COALESCE(outbox_id, ''), COALESCE(employee_id, ''), channel, status, attempts, last_error, created_at, updated_at`, deliveryID).
		Scan(&delivery.DeliveryID, &delivery.OutboxID, &employeeID, &delivery.Channel, &delivery.Status, &delivery.Attempts, &delivery.LastError, &delivery.CreatedAt, &delivery.UpdatedAt)
	if err != nil {
		return NotificationDelivery{}, err
	}
	delivery.EmployeeRef = notificationEmployeeRef(employeeID)
	delivery.LastError = redactNotificationDeliveryError(delivery.LastError, employeeID)
	if _, err := tx.Exec(ctx, `UPDATE outbox_events
		SET publish_status = 'pending',
			attempts = 0,
			retry_count = 0,
			available_at = now(),
			last_error = '',
			dead_letter_at = NULL,
			lease_started_at = NULL
		WHERE outbox_id = $1`, delivery.OutboxID); err != nil {
		return NotificationDelivery{}, err
	}
	auditID, err := newID("aud")
	if err != nil {
		return NotificationDelivery{}, err
	}
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor, "notification.delivery.retry", "notification_delivery", delivery.DeliveryID, map[string]interface{}{
		"outbox_id":           outboxID,
		"channel":             channel,
		"previous_status":     previousStatus,
		"next_status":         delivery.Status,
		"retry_budget_reset":  true,
		"dead_letter_cleared": deadLetterCleared,
	})); err != nil {
		return NotificationDelivery{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return NotificationDelivery{}, err
	}
	s.logNotificationDeliveryRetryQueued(ctx, actor.Role, delivery.DeliveryID, outboxID, channel, previousStatus, delivery.Status, deadLetterCleared)
	return delivery, nil
}

func (s *Service) logNotificationDeliveryRetryQueued(ctx context.Context, actorRole, deliveryID, outboxID, channel, previousStatus, nextStatus string, deadLetterCleared bool) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Info("notification delivery retry queued",
		"trace_id", traceid.FromContext(ctx),
		"action", "notification.delivery.retry",
		"actor_role", actorRole,
		"delivery_id", deliveryID,
		"outbox_id", outboxID,
		"worker_kind", outboxWorkerKindNotification,
		"channel", channel,
		"previous_status", previousStatus,
		"next_status", nextStatus,
		"retry_budget_reset", true,
		"dead_letter_cleared", deadLetterCleared,
	)
}

func (s *Service) findNotificationPreferences(ctx context.Context, employeeID string) (NotificationPreferences, error) {
	var prefs NotificationPreferences
	var categories string
	err := s.db.QueryRow(ctx, `SELECT employee_id, email_enabled, in_app_enabled, opted_out_categories, updated_at
		FROM notification_preferences WHERE employee_id = $1`, employeeID).
		Scan(&prefs.EmployeeID, &prefs.EmailEnabled, &prefs.InAppEnabled, &categories, &prefs.UpdatedAt)
	if err != nil {
		return NotificationPreferences{}, err
	}
	prefs.OptedOutCategories = splitTags(categories)
	return prefs, nil
}

func notificationEmployeeRef(employeeID string) string {
	employeeID = strings.TrimSpace(employeeID)
	if employeeID == "" {
		return ""
	}
	return maskID(employeeID)
}
