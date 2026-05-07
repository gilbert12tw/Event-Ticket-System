package httpapi

import (
	"net/http"

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
