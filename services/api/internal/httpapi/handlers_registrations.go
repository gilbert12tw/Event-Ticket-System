package httpapi

import (
	"net/http"

	"event-ticket-system/internal/ticketing"
)

func handleBook(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.BookingRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.Book(r.Context(), actorFromRequest(r), r.PathValue("event_id"), req)
		writeServiceResult(w, http.StatusCreated, result, err)
	}
}

func handleListRegistrations(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.ListRegistrations(r.Context(), actorFromRequest(r), r.PathValue("event_id"))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleCancelRegistration(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.CancelRegistrationRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.CancelRegistration(r.Context(), actorFromRequest(r), r.PathValue("event_id"), r.PathValue("registration_id"), req)
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handlePromoteWaitlist(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.PromoteWaitlist(r.Context(), actorFromRequest(r), r.PathValue("event_id"))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleRunLottery(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.LotteryRunRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.RunLottery(r.Context(), actorFromRequest(r), r.PathValue("event_id"), req)
		writeServiceResult(w, http.StatusCreated, result, err)
	}
}
