package httpapi

import (
	"net/http"
	"time"

	"event-ticket-system/internal/ticketing"
)

func handleOpsQueues(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.OutboxQueueStatus(r.Context(), actorFromRequest(r))
		writeOpsResult(w, result, err, operationalOpsMeta())
	}
}

func handleOpsCapacityPressure(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.CapacityPressure(r.Context(), actorFromRequest(r))
		writeOpsResult(w, result, err, operationalOpsMeta())
	}
}

func handleOpsReportFreshness(service TicketingService, thresholdSeconds int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.ReportFreshness(r.Context(), actorFromRequest(r), thresholdSeconds)
		writeOpsResult(w, result, err, reportFreshnessOpsMeta(result))
	}
}

func handleOpsDashboard(service TicketingService, thresholdSeconds int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.OpsDashboard(r.Context(), actorFromRequest(r), thresholdSeconds)
		writeOpsResult(w, result, err, dashboardOpsMeta(result))
	}
}

type opsMeta struct {
	AsOf       *time.Time `json:"as_of"`
	Source     string     `json:"source"`
	LagSeconds int        `json:"lag_seconds,omitempty"`
	Degraded   bool       `json:"degraded,omitempty"`
}

func writeOpsResult(w http.ResponseWriter, result interface{}, err error, meta opsMeta) {
	if err != nil {
		writeErrorWithCode(w, ticketing.ErrorStatus(err), ticketing.ErrorMessage(err), ticketing.ErrorCode(err))
		return
	}
	writeJSONWithMeta(w, http.StatusOK, result, meta)
}

func operationalOpsMeta() opsMeta {
	now := time.Now().UTC()
	return opsMeta{AsOf: &now, Source: "operational"}
}

func dashboardOpsMeta(result ticketing.OpsDashboard) opsMeta {
	if len(result.ReportsFreshness.Projections) == 0 {
		return operationalOpsMeta()
	}
	return reportFreshnessOpsMeta(result.ReportsFreshness)
}

func reportFreshnessOpsMeta(result ticketing.ReportFreshness) opsMeta {
	meta := opsMeta{Source: "reporting_projection"}
	for _, projection := range result.Projections {
		if projection.LastApplied != nil && (meta.AsOf == nil || projection.LastApplied.After(*meta.AsOf)) {
			at := *projection.LastApplied
			meta.AsOf = &at
		}
		if projection.LagSeconds > meta.LagSeconds {
			meta.LagSeconds = projection.LagSeconds
		}
		meta.Degraded = meta.Degraded || projection.Degraded
	}
	if meta.AsOf == nil {
		meta.Source = "unavailable"
	}
	return meta
}
