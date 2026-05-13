package httpapi

import (
	"net/http"

	"event-ticket-system/internal/ticketing"
)

func handleListTickets(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.ListTickets(r.Context(), actorFromRequest(r), "")
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleGetTicket(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.GetTicket(r.Context(), actorFromRequest(r), r.PathValue("ticket_id"))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleRevokeTicket(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.RevokeTicketRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.RevokeTicket(r.Context(), actorFromRequest(r), r.PathValue("ticket_id"), req)
		writeServiceResult(w, http.StatusOK, result, err)
	}
}
