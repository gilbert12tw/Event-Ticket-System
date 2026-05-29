package ticketing

import (
	"fmt"
	"strings"
)

func outboxV2Metadata(eventType string, aggregateID string, outboxID string, payload map[string]interface{}) (string, string, error) {
	eventType = strings.TrimSpace(eventType)
	switch eventType {
	case "registration.confirmed.v2":
		return metadataWithOneID(eventType, payload, "registration.confirmed", "registration_id", "event_id")
	case "registration.cancelled.v2":
		return metadataWithTwoIDs(eventType, payload, "registration.cancelled", "registration_id", "cancelled_at", "event_id")
	case "registration.waitlisted.v2":
		return metadataWithOneID(eventType, payload, "registration.waitlisted", "registration_id", "event_id")
	case "registration.promoted.v2":
		return metadataWithTwoIDs(eventType, payload, "registration.promoted", "registration_id", "promoted_at", "event_id")
	case "ticket.issued.v2":
		return metadataWithOneID(eventType, payload, "ticket.issued", "ticket_id", "event_id")
	case "ticket.revoked.v2":
		return metadataWithOneID(eventType, payload, "ticket.revoked", "ticket_id", "event_id")
	case "ticket.expired.v2":
		return metadataWithOneID(eventType, payload, "ticket.expired", "ticket_id", "event_id")
	case "checkin.recorded.v2":
		return metadataWithOneID(eventType, payload, "checkin.recorded", "ticket_id", "event_id")
	case "notification.requested.v2":
		return metadataWithOneID(eventType, payload, "notification.requested", "notification_id", "recipient_employee_id")
	case "reservation.compensation.release_required.v2":
		return metadataWithOneID(eventType, payload, "reservation.compensation.release_required", "reservation_id", "event_id")
	case outboxEventReportExportRequestedV2:
		return metadataWithOneID(eventType, payload, "report.export.requested", "export_id", "export_id")
	case "report.export.completed.v2":
		return metadataWithOneID(eventType, payload, "report.export.completed", "export_id", "export_id")
	case "report.export.failed.v2":
		return metadataWithOneID(eventType, payload, "report.export.failed", "export_id", "export_id")
	case "hr_sync.batch.completed.v2":
		return metadataWithOneID(eventType, payload, "hr_sync.batch.completed", "batch_id", "batch_id")
	case "eligibility.impact_review.created.v2":
		return metadataWithOneID(eventType, payload, "eligibility.impact_review.created", "review_id", "event_id")
	case outboxEventReportingProjectionUpdateRequiredV2:
		projectionName, err := requiredOutboxPayloadString(eventType, payload, "projection_name")
		if err != nil {
			return "", "", err
		}
		aggregateID, err := requiredOutboxPayloadString(eventType, payload, "aggregate_id")
		if err != nil {
			return "", "", err
		}
		triggerEventID, err := requiredOutboxPayloadString(eventType, payload, "trigger_event_id")
		if err != nil {
			return "", "", err
		}
		return "reporting.projection.update_required:" + projectionName + ":" + aggregateID + ":" + triggerEventID, projectionName, nil
	default:
		return eventType + ":" + outboxID, aggregateID, nil
	}
}

func metadataWithOneID(eventType string, payload map[string]interface{}, prefix string, idField string, partitionField string) (string, string, error) {
	id, err := requiredOutboxPayloadString(eventType, payload, idField)
	if err != nil {
		return "", "", err
	}
	partitionKey, err := requiredOutboxPayloadString(eventType, payload, partitionField)
	if err != nil {
		return "", "", err
	}
	return prefix + ":" + id, partitionKey, nil
}

func metadataWithTwoIDs(eventType string, payload map[string]interface{}, prefix string, firstField string, secondField string, partitionField string) (string, string, error) {
	first, err := requiredOutboxPayloadString(eventType, payload, firstField)
	if err != nil {
		return "", "", err
	}
	second, err := requiredOutboxPayloadString(eventType, payload, secondField)
	if err != nil {
		return "", "", err
	}
	partitionKey, err := requiredOutboxPayloadString(eventType, payload, partitionField)
	if err != nil {
		return "", "", err
	}
	return prefix + ":" + first + ":" + second, partitionKey, nil
}

func requiredOutboxPayloadString(eventType string, payload map[string]interface{}, field string) (string, error) {
	value, ok := payload[field].(string)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", fmt.Errorf("outbox event %s requires payload.%s", eventType, field)
	}
	return value, nil
}
