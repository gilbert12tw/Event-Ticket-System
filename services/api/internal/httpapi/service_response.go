package httpapi

import (
	"net/http"

	"event-ticket-system/internal/ticketing"
)

func writeServiceResult(w http.ResponseWriter, successStatus int, result interface{}, err error) {
	if err != nil {
		writeErrorWithCode(w, ticketing.ErrorStatus(err), ticketing.ErrorMessage(err), ticketing.ErrorCode(err))
		return
	}
	writeJSON(w, successStatus, result)
}
