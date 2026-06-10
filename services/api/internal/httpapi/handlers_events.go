package httpapi

import (
	"net/http"
	"strings"

	"event-ticket-system/internal/ticketing"
)

func handleListAdminEvents(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.ListAdminEvents(r.Context(), actorFromRequest(r))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleCreateEvent(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.CreateEventRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.CreateEvent(r.Context(), actorFromRequest(r), req)
		writeServiceResult(w, http.StatusCreated, result, err)
	}
}

func handleGetEvent(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectCallerEmployeeIDQuery(w, r) {
			return
		}
		result, err := service.GetEvent(r.Context(), actorFromRequest(r), r.PathValue("event_id"), "")
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleUpdateEvent(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.UpdateEventRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.UpdateEvent(r.Context(), actorFromRequest(r), r.PathValue("event_id"), req)
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleChangeEventState(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.ChangeEventStateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.ChangeEventState(r.Context(), actorFromRequest(r), r.PathValue("event_id"), req)
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleDuplicateEvent(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.DuplicateEvent(r.Context(), actorFromRequest(r), r.PathValue("event_id"))
		writeServiceResult(w, http.StatusCreated, result, err)
	}
}

func handleArchiveEvent(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.ArchiveEvent(r.Context(), actorFromRequest(r), r.PathValue("event_id"))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleListEvents(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectCallerEmployeeIDQuery(w, r) {
			return
		}
		query, ok := eventListQueryFromRequest(w, r)
		if !ok {
			return
		}
		result, err := service.ListEvents(r.Context(), actorFromRequest(r), "", query)
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleHideEvent(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := service.HideEvent(r.Context(), actorFromRequest(r), r.PathValue("event_id"))
		writeServiceStatus(w, http.StatusNoContent, err)
	}
}

func handleUnhideEvent(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := service.UnhideEvent(r.Context(), actorFromRequest(r), r.PathValue("event_id"))
		writeServiceStatus(w, http.StatusNoContent, err)
	}
}

func eventListQueryFromRequest(w http.ResponseWriter, r *http.Request) (ticketing.EventListQuery, bool) {
	params := r.URL.Query()
	status := strings.TrimSpace(params.Get("status"))
	if status != "" && status != ticketing.EventStatusPublished {
		writeError(w, http.StatusBadRequest, "status must be published")
		return ticketing.EventListQuery{}, false
	}
	return ticketing.EventListQuery{
		CapacityType: params.Get("capacity_type"),
		City:         params.Get("city"),
		Status:       status,
	}, true
}
