package httpapi

import (
	"net/http"

	"event-ticket-system/internal/ticketing"
)

func writeServiceResult(w http.ResponseWriter, successStatus int, result interface{}, err error) {
	if err != nil {
		writeError(w, ticketing.ErrorStatus(err), ticketing.ErrorMessage(err))
		return
	}
	writeJSON(w, successStatus, result)
}
