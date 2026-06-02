package httpapi

import (
	"log/slog"
	"net/http"
)

func registerTicketingRoutes(mux *http.ServeMux, service TicketingService, readService TicketingService, appEnv string, provider *ProviderVerifier, opsAPIEnabled bool, reportStaleThresholdSeconds int, logger *slog.Logger) {
	protected := func(routeService TicketingService, next http.HandlerFunc) http.HandlerFunc {
		return requireActor(provider, requireService(routeService, next))
	}
	mux.HandleFunc("GET /api/v1/admin/events", protected(readService, handleListAdminEvents(readService)))
	mux.HandleFunc("POST /api/v1/admin/events", protected(service, handleCreateEvent(service)))
	mux.HandleFunc("GET /api/v1/events", protected(readService, handleListEvents(readService)))
	mux.HandleFunc("GET /api/v1/events/{event_id}", protected(readService, handleGetEvent(readService)))
	mux.HandleFunc("PATCH /api/v1/admin/events/{event_id}", protected(service, handleUpdateEvent(service)))
	mux.HandleFunc("POST /api/v1/admin/events/{event_id}/state", protected(service, handleChangeEventState(service)))
	mux.HandleFunc("POST /api/v1/admin/events/{event_id}/duplicate", protected(service, handleDuplicateEvent(service)))
	mux.HandleFunc("DELETE /api/v1/admin/events/{event_id}", protected(service, handleArchiveEvent(service)))
	mux.HandleFunc("GET /api/v1/events/{event_id}/eligibility", protected(readService, handleEligibility(readService)))
	mux.HandleFunc("POST /api/v1/admin/events/{event_id}/eligibility/preview", protected(readService, handlePreviewEligibility(readService)))
	mux.HandleFunc("PUT /api/v1/admin/events/{event_id}/eligibility", protected(service, handleUpdateEligibility(service)))
	mux.HandleFunc("GET /api/v1/admin/eligibility-impact-reviews", protected(readService, handleEligibilityImpactReviews(readService)))
	mux.HandleFunc("POST /api/v1/admin/eligibility-impact-reviews/{review_id}/resolve", protected(service, handleResolveEligibilityImpactReview(service)))
	mux.HandleFunc("POST /api/v1/events/{event_id}/bookings", protected(service, handleBook(service)))
	mux.HandleFunc("GET /api/v1/admin/events/{event_id}/registrations", protected(readService, handleListRegistrations(readService)))
	mux.HandleFunc("POST /api/v1/me/registrations/{registration_id}/cancel", protected(service, handleCancelMyRegistration(service)))
	mux.HandleFunc("POST /api/v1/events/{event_id}/bookings/{registration_id}/cancel", protected(service, handleCancelRegistration(service)))
	mux.HandleFunc("POST /api/v1/admin/events/{event_id}/registrations/{registration_id}/cancel", protected(service, handleCancelRegistration(service)))
	mux.HandleFunc("POST /api/v1/admin/events/{event_id}/waitlist/promote", protected(service, handlePromoteWaitlist(service)))
	mux.HandleFunc("DELETE /api/v1/admin/events/{event_id}/bans/{target_employee_id}", protected(service, handleLiftBookingBan(service)))
	mux.HandleFunc("POST /api/v1/admin/events/{event_id}/lottery-runs", protected(service, handleRunLottery(service)))
	mux.HandleFunc("GET /api/v1/me/tickets", protected(readService, handleListTickets(readService)))
	mux.HandleFunc("GET /api/v1/tickets/{ticket_id}", protected(readService, handleGetTicket(readService)))
	mux.HandleFunc("POST /api/v1/admin/tickets/{ticket_id}/revoke", protected(service, handleRevokeTicket(service)))
	mux.HandleFunc("POST /api/v1/checkins", protected(service, handleCheckin(service)))
	mux.HandleFunc("GET /api/v1/checkins/events/{event_id}/offline-package", protected(readService, handleOfflineCheckinPackage(readService)))
	mux.HandleFunc("POST /api/v1/checkins/offline-sync", protected(service, handleOfflineCheckinSync(service)))
	mux.HandleFunc("GET /api/v1/notifications/preferences", protected(readService, handleGetNotificationPreferences(readService)))
	mux.HandleFunc("PUT /api/v1/notifications/preferences", protected(service, handleUpdateNotificationPreferences(service)))
	mux.HandleFunc("GET /api/v1/admin/notifications/deliveries", protected(readService, handleNotificationDeliveries(readService)))
	mux.HandleFunc("POST /api/v1/admin/notifications/deliveries/{delivery_id}/retry", protected(service, handleRetryNotificationDelivery(service)))
	if opsAPIEnabled {
		mux.HandleFunc("GET /api/v1/admin/ops/capacity-pressure", protected(readService, handleOpsCapacityPressure(readService)))
		mux.HandleFunc("GET /api/v1/admin/ops/queues", protected(readService, handleOpsQueues(readService)))
		mux.HandleFunc("GET /api/v1/admin/ops/notification-deliveries", protected(readService, handleOpsNotificationDeliveries(readService)))
		mux.HandleFunc("GET /api/v1/admin/ops/report-freshness", protected(readService, handleOpsReportFreshness(readService, reportStaleThresholdSeconds)))
		mux.HandleFunc("GET /api/v1/admin/ops/dashboard", protected(readService, handleOpsDashboard(readService, reportStaleThresholdSeconds)))
	}
	mux.HandleFunc("GET /api/v1/admin/reports", protected(readService, handleReports(readService, reportStaleThresholdSeconds, logger)))
	mux.HandleFunc("POST /api/v1/admin/reports/exports", protected(service, handleCreateReportExport(service)))
	mux.HandleFunc("GET /api/v1/admin/reports/exports/{export_id}", protected(readService, handleGetReportExport(readService)))
	mux.HandleFunc("GET /api/v1/admin/audit-logs", protected(readService, handleAuditLogs(readService)))
	seedDemo := requireService(service, handleSeedDemo(service, appEnv))
	if mockProfilesEnabled(appEnv) {
		seedDemo = requireActor(provider, seedDemo)
	}
	mux.HandleFunc("POST /api/v1/admin/seed-demo", seedDemo)
}

func requireService(service TicketingService, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusServiceUnavailable, "ticketing service is not configured")
			return
		}
		next(w, r)
	}
}
