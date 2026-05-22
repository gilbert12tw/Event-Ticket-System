package httpapi

import "net/http"

func registerTicketingRoutes(mux *http.ServeMux, service TicketingService, appEnv string, provider *ProviderVerifier) {
	protected := func(next http.HandlerFunc) http.HandlerFunc {
		return requireActor(provider, requireService(service, next))
	}
	mux.HandleFunc("GET /api/v1/admin/events", protected(handleListAdminEvents(service)))
	mux.HandleFunc("POST /api/v1/admin/events", protected(handleCreateEvent(service)))
	mux.HandleFunc("GET /api/v1/events", protected(handleListEvents(service)))
	mux.HandleFunc("GET /api/v1/events/{event_id}", protected(handleGetEvent(service)))
	mux.HandleFunc("PATCH /api/v1/admin/events/{event_id}", protected(handleUpdateEvent(service)))
	mux.HandleFunc("POST /api/v1/admin/events/{event_id}/state", protected(handleChangeEventState(service)))
	mux.HandleFunc("POST /api/v1/admin/events/{event_id}/duplicate", protected(handleDuplicateEvent(service)))
	mux.HandleFunc("DELETE /api/v1/admin/events/{event_id}", protected(handleArchiveEvent(service)))
	mux.HandleFunc("GET /api/v1/events/{event_id}/eligibility", protected(handleEligibility(service)))
	mux.HandleFunc("POST /api/v1/admin/events/{event_id}/eligibility/preview", protected(handlePreviewEligibility(service)))
	mux.HandleFunc("PUT /api/v1/admin/events/{event_id}/eligibility", protected(handleUpdateEligibility(service)))
	mux.HandleFunc("GET /api/v1/admin/eligibility-impact-reviews", protected(handleEligibilityImpactReviews(service)))
	mux.HandleFunc("POST /api/v1/admin/eligibility-impact-reviews/{review_id}/resolve", protected(handleResolveEligibilityImpactReview(service)))
	mux.HandleFunc("POST /api/v1/events/{event_id}/bookings", protected(handleBook(service)))
	mux.HandleFunc("GET /api/v1/admin/events/{event_id}/registrations", protected(handleListRegistrations(service)))
	mux.HandleFunc("POST /api/v1/me/registrations/{registration_id}/cancel", protected(handleCancelMyRegistration(service)))
	mux.HandleFunc("POST /api/v1/events/{event_id}/bookings/{registration_id}/cancel", protected(handleCancelRegistration(service)))
	mux.HandleFunc("POST /api/v1/admin/events/{event_id}/registrations/{registration_id}/cancel", protected(handleCancelRegistration(service)))
	mux.HandleFunc("POST /api/v1/admin/events/{event_id}/waitlist/promote", protected(handlePromoteWaitlist(service)))
	mux.HandleFunc("DELETE /api/v1/admin/events/{event_id}/bans/{target_employee_id}", protected(handleLiftBookingBan(service)))
	mux.HandleFunc("POST /api/v1/admin/events/{event_id}/lottery-runs", protected(handleRunLottery(service)))
	mux.HandleFunc("GET /api/v1/me/tickets", protected(handleListTickets(service)))
	mux.HandleFunc("GET /api/v1/tickets/{ticket_id}", protected(handleGetTicket(service)))
	mux.HandleFunc("POST /api/v1/admin/tickets/{ticket_id}/revoke", protected(handleRevokeTicket(service)))
	mux.HandleFunc("POST /api/v1/checkins", protected(handleCheckin(service)))
	mux.HandleFunc("GET /api/v1/checkins/events/{event_id}/offline-package", protected(handleOfflineCheckinPackage(service)))
	mux.HandleFunc("POST /api/v1/checkins/offline-sync", protected(handleOfflineCheckinSync(service)))
	mux.HandleFunc("GET /api/v1/notifications/preferences", protected(handleGetNotificationPreferences(service)))
	mux.HandleFunc("PUT /api/v1/notifications/preferences", protected(handleUpdateNotificationPreferences(service)))
	mux.HandleFunc("GET /api/v1/admin/notifications/deliveries", protected(handleNotificationDeliveries(service)))
	mux.HandleFunc("POST /api/v1/admin/notifications/deliveries/{delivery_id}/retry", protected(handleRetryNotificationDelivery(service)))
	mux.HandleFunc("GET /api/v1/admin/reports", protected(handleReports(service)))
	mux.HandleFunc("POST /api/v1/admin/reports/exports", protected(handleCreateReportExport(service)))
	mux.HandleFunc("GET /api/v1/admin/reports/exports/{export_id}", protected(handleGetReportExport(service)))
	mux.HandleFunc("GET /api/v1/admin/audit-logs", protected(handleAuditLogs(service)))
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
