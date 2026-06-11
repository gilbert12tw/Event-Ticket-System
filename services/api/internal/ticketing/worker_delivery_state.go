package ticketing

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func ensureNotificationDelivery(ctx context.Context, tx pgx.Tx, outboxID string, employeeID string, channel string, status string, lastError string) (notificationDeliveryState, error) {
	deliveryID, err := newID("del")
	if err != nil {
		return notificationDeliveryState{}, err
	}
	var current notificationDeliveryState
	err = tx.QueryRow(ctx, `INSERT INTO notification_deliveries
		(delivery_id, outbox_id, employee_id, channel, status, attempts, last_error)
		VALUES ($1,$2,$3,$4,$5,0,$6)
		ON CONFLICT (outbox_id, channel) WHERE outbox_id IS NOT NULL DO UPDATE SET
			employee_id = EXCLUDED.employee_id,
			status = CASE
				WHEN notification_deliveries.status IN ('sending', 'sent', 'suppressed') THEN notification_deliveries.status
				ELSE EXCLUDED.status
			END,
			last_error = CASE
				WHEN notification_deliveries.status IN ('sending', 'sent', 'suppressed') THEN notification_deliveries.last_error
				ELSE EXCLUDED.last_error
			END,
			updated_at = CASE
				WHEN notification_deliveries.status IN ('sending', 'sent', 'suppressed') THEN notification_deliveries.updated_at
				ELSE now()
			END
		RETURNING delivery_id, status`, deliveryID, outboxID, employeeID, channel, status, lastError).
		Scan(&current.deliveryID, &current.status)
	return current, err
}

func markOutboxPublishedInTx(ctx context.Context, tx pgx.Tx, claim outboxClaim) error {
	return markOutboxPublishedWith(ctx, tx, claim)
}

func (s *Service) markOutboxPublished(ctx context.Context, claim outboxClaim) error {
	return markOutboxPublishedWith(ctx, s.db, claim)
}

func markOutboxPublishedWith(ctx context.Context, exec sqlExecutor, claim outboxClaim) error {
	tag, err := exec.Exec(ctx, `UPDATE outbox_events
		SET publish_status = 'published',
			last_error = '',
			lease_started_at = NULL,
			published_at = now()
		WHERE outbox_id = $1
			AND publish_status = 'processing'
			AND lease_started_at = $2`, claim.outboxID, claim.leaseStartedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errOutboxLeaseLost
	}
	return nil
}

func (s *Service) updateEmailDeliveryAfterSend(ctx context.Context, outboxID string, status string, lastError string) error {
	tag, err := s.db.Exec(ctx, `UPDATE notification_deliveries
		SET status = $1,
			last_error = $2,
			updated_at = now()
		WHERE outbox_id = $3 AND channel = 'email' AND status = 'sending'`, status, lastError, outboxID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("email delivery send acknowledgement was not recorded")
	}
	return err
}

func (s *Service) markEmailDeliverySending(ctx context.Context, deliveryID string) error {
	tag, err := s.db.Exec(ctx, `UPDATE notification_deliveries
		SET status = 'sending',
			attempts = attempts + 1,
			last_error = '',
			updated_at = now()
		WHERE delivery_id = $1 AND status = 'pending'`, deliveryID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("email delivery send attempt was not recorded")
	}
	return nil
}
