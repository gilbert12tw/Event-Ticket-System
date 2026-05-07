package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"event-ticket-system/internal/ticketing"
)

func handleAuditLogs(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := auditLogQueryFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.AuditLogs(r.Context(), actorFromRequest(r), query)
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func auditLogQueryFromRequest(r *http.Request) (ticketing.AuditLogQuery, error) {
	values := r.URL.Query()
	limit, _ := strconv.Atoi(values.Get("limit"))
	cursor, cursorID, err := ticketingAuditCursor(values.Get("cursor"))
	if err != nil {
		return ticketing.AuditLogQuery{}, err
	}
	from, err := parseOptionalTime(values.Get("from"))
	if err != nil {
		return ticketing.AuditLogQuery{}, err
	}
	to, err := parseOptionalTime(values.Get("to"))
	if err != nil {
		return ticketing.AuditLogQuery{}, err
	}
	return ticketing.AuditLogQuery{
		ActorID:    strings.TrimSpace(values.Get("actor_id")),
		Role:       strings.TrimSpace(values.Get("role")),
		Action:     strings.TrimSpace(values.Get("action")),
		EntityType: strings.TrimSpace(values.Get("entity_type")),
		EntityID:   strings.TrimSpace(values.Get("entity_id")),
		From:       from,
		To:         to,
		Cursor:     cursor,
		CursorID:   cursorID,
		Limit:      limit,
	}, nil
}

func ticketingAuditCursor(raw string) (time.Time, string, error) {
	return ticketing.ParseAuditCursorStrict(raw)
}

func parseOptionalTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}
