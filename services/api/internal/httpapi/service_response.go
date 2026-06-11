package httpapi

import (
	"net/http"
	"strconv"

	"event-ticket-system/internal/ticketing"
)

func writeServiceResult(w http.ResponseWriter, successStatus int, result interface{}, err error) {
	if writeServiceError(w, err) {
		return
	}
	writeJSON(w, successStatus, result)
}

// writeServiceStatus maps a service error like writeServiceResult, but writes an empty body with
// successStatus when there is no result payload (e.g. 204 No Content for toggle-style actions).
func writeServiceStatus(w http.ResponseWriter, successStatus int, err error) {
	if writeServiceError(w, err) {
		return
	}
	w.WriteHeader(successStatus)
}

// writeServiceError maps a non-nil service error (including its Retry-After
// hint) onto the response and reports whether it handled one.
func writeServiceError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if retryAfter, ok := ticketing.ErrorRetryAfterSeconds(err); ok {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	}
	writeErrorWithCode(w, ticketing.ErrorStatus(err), ticketing.ErrorMessage(err), ticketing.ErrorCode(err))
	return true
}
