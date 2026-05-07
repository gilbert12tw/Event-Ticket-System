package httpapi

import (
	"net/http"
	"strings"

	"event-ticket-system/internal/ticketing"
)

func handleEligibility(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.CheckEligibility(r.Context(), actorFromRequest(r), r.PathValue("event_id"), strings.TrimSpace(r.URL.Query().Get("employee_id")))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handlePreviewEligibility(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.EligibilityPreviewRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.PreviewEligibility(r.Context(), actorFromRequest(r), r.PathValue("event_id"), req)
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleUpdateEligibility(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.UpdateEligibilityRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.UpdateEligibility(r.Context(), actorFromRequest(r), r.PathValue("event_id"), req)
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleEligibilityImpactReviews(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := service.EligibilityImpactReviews(r.Context(), actorFromRequest(r))
		writeServiceResult(w, http.StatusOK, result, err)
	}
}

func handleResolveEligibilityImpactReview(service TicketingService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ticketing.ResolveImpactReviewRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		result, err := service.ResolveEligibilityImpactReview(r.Context(), actorFromRequest(r), r.PathValue("review_id"), req)
		writeServiceResult(w, http.StatusOK, result, err)
	}
}
