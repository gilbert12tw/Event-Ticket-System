package ticketing

import "strings"

const (
	outboxWorkerKindNotification = "notification"
	outboxWorkerKindProjection   = "projection"
	outboxWorkerKindCompensation = "compensation"
	outboxWorkerKindExport       = "export"
	outboxWorkerKindUnknown      = "unknown"

	outboxEventReportExportRequested               = "report.export.requested"
	outboxEventReportExportRequestedV2             = "report.export.requested.v2"
	outboxEventReportingProjectionUpdateRequiredV2 = "reporting.projection.update_required.v2"
)

func outboxWorkerKindForEventType(eventType string) string {
	eventType = strings.TrimSpace(eventType)
	switch {
	case isReportExportRequestedEventType(eventType):
		return outboxWorkerKindExport
	case eventType == outboxEventReportingProjectionUpdateRequiredV2:
		return outboxWorkerKindProjection
	case strings.HasPrefix(eventType, "reservation.compensation."):
		return outboxWorkerKindCompensation
	default:
		return outboxWorkerKindNotification
	}
}

func retryableNotificationOutboxEventType(eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	return knownReplayEventType(eventType) &&
		!unsafeOutboxTelemetryEventType(eventType) &&
		outboxWorkerKindForEventType(eventType) == outboxWorkerKindNotification
}

func isReportExportRequestedEventType(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case outboxEventReportExportRequested, outboxEventReportExportRequestedV2:
		return true
	default:
		return false
	}
}

func outboxWorkerKindFilter(kinds []string) []string {
	if len(kinds) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	filter := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		kind = strings.ToLower(strings.TrimSpace(kind))
		if kind == "" {
			continue
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		filter = append(filter, kind)
	}
	return filter
}
