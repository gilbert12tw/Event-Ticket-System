package httpapi

import (
	"net/http"

	"event-ticket-system/internal/ticketing"
)

func handleReports(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.Reports(r.Context(), actorFromRequest(r))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleCreateReportExport(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.ReportExportRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.CreateReportExport(r.Context(), actorFromRequest(r), req)
		writeServiceResult(w, http.StatusCreated, result, err)
	}
}

func handleGetReportExport(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.GetReportExport(r.Context(), actorFromRequest(r), r.PathValue("export_id"))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}
