package ticketing

import (
	"regexp"
	"strings"
)

var notificationEmailPattern = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)

func redactNotificationDeliveryError(raw string, employeeID string) string {
	message := strings.TrimSpace(raw)
	if message == "" {
		return ""
	}
	message = notificationEmailPattern.ReplaceAllString(message, "[redacted email]")
	employeeID = strings.TrimSpace(employeeID)
	if employeeID == "" {
		return message
	}
	return regexp.MustCompile(`(?i)`+regexp.QuoteMeta(employeeID)).ReplaceAllString(message, maskID(employeeID))
}
