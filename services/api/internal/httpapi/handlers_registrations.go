package httpapi

import (
	"net/http"
	"strings"

	"event-ticket-system/internal/ticketing"
)

func handleBook(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.BookingRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.TrimSpace(req.EmployeeID) != "" {
			writeError(w, http.StatusBadRequest, callerEmployeeIDMessage)
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

func handleCancelMyRegistration(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.CancelRegistrationRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.CancelMyRegistration(r.Context(), actorFromRequest(r), r.PathValue("registration_id"), req)
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

func handleLiftBookingBan(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := service.LiftBookingBan(r.Context(), actorFromRequest(r), r.PathValue("event_id"), r.PathValue("target_employee_id"))
		if err != nil {
			writeErrorWithCode(w, ticketing.ErrorStatus(err), ticketing.ErrorMessage(err), ticketing.ErrorCode(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
