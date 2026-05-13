package httpapi

import "net/http"

const callerEmployeeIDMessage = "employee_id is derived from provider claims"

func rejectCallerEmployeeIDQuery(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := r.URL.Query()["employee_id"]; ok {
		writeError(w, http.StatusBadRequest, callerEmployeeIDMessage)
		return true
	}
	return false
}
