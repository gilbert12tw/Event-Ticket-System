package ticketing

import (
	"regexp"
	"strings"
)

var (
	notificationEmailPattern     = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	notificationEmailBodyPattern = regexp.MustCompile(`(?i)\b(email[\s_-]*body|message[\s_-]*body|body)\s*[:=]\s*.*$`)
)

func redactNotificationDeliveryError(raw string, employeeID string) string {
	message := strings.TrimSpace(raw)
	if message == "" {
		return ""
	}
	message = notificationEmailBodyPattern.ReplaceAllString(message, "$1=[redacted email body]")
	message = notificationEmailPattern.ReplaceAllString(message, "[redacted email]")
	message = outboxTelemetryTokenPattern.ReplaceAllString(message, "[redacted token]")
	employeeID = strings.TrimSpace(employeeID)
	if employeeID == "" {
		return message
	}
	return regexp.MustCompile(`(?i)`+regexp.QuoteMeta(employeeID)).ReplaceAllString(message, maskID(employeeID))
}
