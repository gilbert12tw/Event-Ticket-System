package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"event-ticket-system/internal/ticketing"
)

func handleGetNotificationPreferences(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.GetNotificationPreferences(r.Context(), actorFromRequest(r))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleUpdateNotificationPreferences(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.NotificationPreferences
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.UpdateNotificationPreferences(r.Context(), actorFromRequest(r), req)
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleNotificationDeliveries(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.NotificationDeliveries(r.Context(), actorFromRequest(r))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleRetryNotificationDelivery(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.RetryNotificationDelivery(r.Context(), actorFromRequest(r), r.PathValue("delivery_id"))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleOpsNotificationDeliveries(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := notificationDeliveryOpsQueryFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.NotificationDeliveryOpsFeed(r.Context(), actorFromRequest(r), query)
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func notificationDeliveryOpsQueryFromRequest(r *http.Request) (ticketing.NotificationDeliveryOpsQuery, error) {
	values := r.URL.Query()
	limit, err := optionalIntQuery(values.Get("limit"), "notification delivery limit")
	if err != nil {
		return ticketing.NotificationDeliveryOpsQuery{}, err
	}
	cursor, cursorID, err := ticketing.ParseNotificationDeliveryCursorStrict(values.Get("cursor"))
	if err != nil {
		return ticketing.NotificationDeliveryOpsQuery{}, err
	}
	return ticketing.NotificationDeliveryOpsQuery{
		Status:   strings.TrimSpace(values.Get("status")),
		Limit:    limit,
		Cursor:   cursor,
		CursorID: cursorID,
	}, nil
}

func optionalIntQuery(raw string, label string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid %s", label)
	}
	return parsed, nil
}
