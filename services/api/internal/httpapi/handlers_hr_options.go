package httpapi

import "net/http"

func handleAdminHROptions(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.AdminHROptions(r.Context(), actorFromRequest(r))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}
