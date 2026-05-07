package httpapi

import (
	"net/http"
	"strings"

	"event-ticket-system/internal/ticketing"
)

func handleCheckin(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.CheckinRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.CheckIn(r.Context(), actorFromRequest(r), req)
		if err != nil && result.CheckinID != "" {
			writeErrorWithData(w, ticketing.ErrorStatus(err), result, ticketing.ErrorMessage(err))
			return
		}
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleOfflineCheckinPackage(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
		result, err := service.OfflineCheckinPackage(r.Context(), actorFromRequest(r), r.PathValue("event_id"), deviceID)
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleOfflineCheckinSync(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.OfflineCheckinSyncRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.SyncOfflineCheckins(r.Context(), actorFromRequest(r), req)
		writeServiceResult(w, http.StatusOK, result, err)
	}
}
