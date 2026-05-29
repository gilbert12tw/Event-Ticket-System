package ticketing

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

const (
	defaultNotificationDeliveryOpsLimit = 100
	maxNotificationDeliveryOpsLimit     = 200
)

func effectiveNotificationDeliveryOpsQuery(query []NotificationDeliveryOpsQuery) (NotificationDeliveryOpsQuery, error) {
	q := NotificationDeliveryOpsQuery{Limit: defaultNotificationDeliveryOpsLimit}
	if len(query) > 0 {
		q = query[0]
	}
	q.Status = strings.TrimSpace(q.Status)
	q.CursorID = strings.TrimSpace(q.CursorID)
	if q.Limit < 0 {
		return NotificationDeliveryOpsQuery{}, badRequest("notification delivery limit is invalid")
	}
	if q.Limit <= 0 {
		q.Limit = defaultNotificationDeliveryOpsLimit
	}
	if q.Limit > maxNotificationDeliveryOpsLimit {
		q.Limit = maxNotificationDeliveryOpsLimit
	}
	if !notificationDeliveryStatusAllowed(q.Status) {
		return NotificationDeliveryOpsQuery{}, badRequest("notification delivery status is invalid")
	}
	if !q.Cursor.IsZero() && q.CursorID == "" {
		return NotificationDeliveryOpsQuery{}, badRequest("notification delivery cursor is invalid")
	}
	return q, nil
}

func notificationDeliveryStatusAllowed(status string) bool {
	switch status {
	case "", deliveryStatusPending, deliveryStatusSending, deliveryStatusSent, deliveryStatusFailed, deliveryStatusSuppressed, deliveryStatusDeadLetter:
		return true
	default:
		return false
	}
}

func EncodeNotificationDeliveryCursor(updatedAt time.Time, deliveryID string) string {
	raw := fmt.Sprintf("%s|%s", updatedAt.UTC().Format(time.RFC3339Nano), strings.TrimSpace(deliveryID))
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func ParseNotificationDeliveryCursorStrict(raw string) (time.Time, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, "", nil
	}
	cursor, cursorID := parseNotificationDeliveryCursor(raw)
	if cursor.IsZero() || strings.TrimSpace(cursorID) == "" {
		return time.Time{}, "", fmt.Errorf("invalid notification delivery cursor")
	}
	return cursor, strings.TrimSpace(cursorID), nil
}

func parseNotificationDeliveryCursor(raw string) (time.Time, string) {
	if strings.Contains(raw, "|") {
		parts := strings.SplitN(raw, "|", 2)
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(parts[0]))
		if err != nil {
			return time.Time{}, ""
		}
		return parsed, strings.TrimSpace(parts[1])
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err == nil && strings.Contains(string(decoded), "|") {
		return parseNotificationDeliveryCursor(string(decoded))
	}
	return time.Time{}, ""
}
