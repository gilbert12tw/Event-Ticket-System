package ticketing

import (
	"regexp"
	"strings"
)

var (
	outboxTelemetryTokenPattern    = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]*(?:\.[A-Za-z0-9_-]+){1,2}\b`)
	outboxTelemetryEmployeePattern = regexp.MustCompile(`(?i)\bE[0-9]{4,}\b`)
)

func safeOutboxTelemetryEventType(eventType string) string {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return outboxWorkerKindUnknown
	}
	if unsafeOutboxTelemetryEventType(eventType) {
		return outboxWorkerKindUnknown
	}
	if knownReplayEventType(eventType) {
		return eventType
	}
	return outboxWorkerKindUnknown
}

func safeOutboxTelemetryWorkerKind(eventType string) string {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" || unsafeOutboxTelemetryEventType(eventType) || !knownReplayEventType(eventType) {
		return outboxWorkerKindUnknown
	}
	return outboxWorkerKindForEventType(eventType)
}

func unsafeOutboxTelemetryEventType(eventType string) bool {
	return notificationEmailPattern.MatchString(eventType) ||
		outboxTelemetryTokenPattern.MatchString(eventType) ||
		outboxTelemetryEmployeePattern.MatchString(eventType)
}
