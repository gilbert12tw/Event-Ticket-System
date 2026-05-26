package ticketing

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func eventContextMetadata(event Event) map[string]interface{} {
	metadata := map[string]interface{}{
		"event_id": event.EventID,
	}
	appendMetadataString(metadata, "event_title", event.Title)
	appendMetadataString(metadata, "event_city", event.EventCity)
	appendMetadataString(metadata, "event_site", event.EventSite)
	if !event.StartsAt.IsZero() {
		metadata["starts_at"] = event.StartsAt.UTC().Format(time.RFC3339)
	}
	return metadata
}

func eventContextMetadataTx(ctx context.Context, tx pgx.Tx, eventID string) (map[string]interface{}, error) {
	eventID = strings.TrimSpace(eventID)
	metadata := map[string]interface{}{"event_id": eventID}
	if eventID == "" {
		return metadata, nil
	}
	var event Event
	err := tx.QueryRow(ctx, `SELECT event_id, title, event_city, event_site, starts_at FROM events WHERE event_id = $1`, eventID).
		Scan(&event.EventID, &event.Title, &event.EventCity, &event.EventSite, &event.StartsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return metadata, nil
	}
	if err != nil {
		return nil, err
	}
	return eventContextMetadata(event), nil
}

func mergeMetadata(base map[string]interface{}, fields map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range fields {
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		merged[key] = value
	}
	return merged
}

func appendMetadataString(metadata map[string]interface{}, key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	metadata[key] = value
}

func ticketOutboxPayload(ticket Ticket, fields map[string]interface{}) map[string]interface{} {
	return mergeMetadata(map[string]interface{}{
		"ticket_id":   ticket.TicketID,
		"event_id":    ticket.EventID,
		"employee_id": ticket.EmployeeID,
	}, fields)
}
