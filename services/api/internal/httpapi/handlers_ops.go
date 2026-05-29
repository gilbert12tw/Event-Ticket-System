package httpapi

import (
	"net/http"
)

func handleOpsQueues(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.OutboxQueueStatus(r.Context(), actorFromRequest(r))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}
