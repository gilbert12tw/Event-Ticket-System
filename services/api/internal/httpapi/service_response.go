package httpapi

import (
	"net/http"
	"strconv"

	"event-ticket-system/internal/ticketing"
)

func writeServiceResult(w http.ResponseWriter, successStatus int, result interface{}, err error) {
	if err != nil {
		if retryAfter, ok := ticketing.ErrorRetryAfterSeconds(err); ok {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		}
		writeErrorWithCode(w, ticketing.ErrorStatus(err), ticketing.ErrorMessage(err), ticketing.ErrorCode(err))
		return
	}
	writeJSON(w, successStatus, result)
}
