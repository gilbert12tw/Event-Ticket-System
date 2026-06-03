package httpapi

import (
	"log/slog"
	"net/http"
	"strings"

	"event-ticket-system/internal/ticketing"
)

func handleReports(service TicketingService, threshold int, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.Reports(r.Context(), actorFromRequest(r))
		if err != nil {
			writeServiceResult(w, http.StatusOK, nil, err)
			return
		}

		meta := ticketing.FreshnessFromProjection(result.ProjectionUpdatedAt, threshold)
		rows := result.Rows
		if meta.Source == ticketing.ReportSourceUnavailable {
			rows = zeroReportRows(result.Rows)
		}

		if logger != nil {
			logger.InfoContext(r.Context(), "report served",
				"degraded", meta.Degraded,
				"lag_seconds", meta.LagSeconds,
			)
		}

		writeJSONWithMeta(w, http.StatusOK, rows, meta)
	}
}

func zeroReportRows(rows []ticketing.ReportRow) []ticketing.ReportRow {
	if len(rows) == 0 {
		return rows
	}
	zeroed := make([]ticketing.ReportRow, len(rows))
	for i, row := range rows {
		zeroed[i] = row
		zeroed[i].ConfirmedCount = 0
		zeroed[i].WaitlistCount = 0
		zeroed[i].EmployeeCount = 0
		zeroed[i].FamilyCount = 0
		zeroed[i].TotalAttendeeCount = 0
		zeroed[i].TicketCount = 0
		zeroed[i].CheckinCount = 0
		zeroed[i].CityDistribution = zeroCityDistribution(row.CityDistribution)
		if row.Capacity != nil {
			remaining := *row.Capacity
			zeroed[i].RemainingCapacity = &remaining
		} else {
			zeroed[i].RemainingCapacity = nil
		}
	}
	return zeroed
}

func zeroCityDistribution(distribution map[string]int) map[string]int {
	if len(distribution) == 0 {
		return map[string]int{}
	}
	zeroed := make(map[string]int, len(distribution))
	for key := range distribution {
		zeroed[key] = 0
	}
	return zeroed
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

func handleDownloadReportExport(service TicketingService, store ticketing.ReportObjectReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			writeError(w, http.StatusServiceUnavailable, "report export object store is not configured")
			return
		}
		export, err := service.GetReportExport(r.Context(), actorFromRequest(r), r.PathValue("export_id"))
		if err != nil {
			writeServiceResult(w, http.StatusOK, nil, err)
			return
		}
		if export.Status != ticketing.ReportExportStatusReady {
			writeError(w, http.StatusConflict, "report export is not ready")
			return
		}
		body, contentType, err := store.Get(r.Context(), export.ObjectKey)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "report export artifact is unavailable")
			return
		}
		if strings.TrimSpace(contentType) == "" {
			contentType = "text/csv; charset=utf-8"
		}
		w.Header().Set(contentTypeHeader, contentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+export.ExportID+`.csv"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}
