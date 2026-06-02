package httpapi

import (
	"context"

	"event-ticket-system/internal/ticketing"
)

type TicketingService interface {
	EventService
	EventAssetService
	EligibilityService
	RegistrationService
	TicketService
	CheckinService
	NotificationService
	ReportingService
	AuditService
	HRMetadataService
	OpsService
	DemoService
}

type EventService interface {
	CreateEvent(ctx context.Context, actor ticketing.Actor, req ticketing.CreateEventRequest) (ticketing.EventSummary, error)
	ListAdminEvents(ctx context.Context, actor ticketing.Actor) ([]ticketing.EventSummary, error)
	GetEvent(ctx context.Context, actor ticketing.Actor, eventID string, employeeID string) (ticketing.EventSummary, error)
	UpdateEvent(ctx context.Context, actor ticketing.Actor, eventID string, req ticketing.UpdateEventRequest) (ticketing.EventSummary, error)
	ChangeEventState(ctx context.Context, actor ticketing.Actor, eventID string, req ticketing.ChangeEventStateRequest) (ticketing.EventSummary, error)
	DuplicateEvent(ctx context.Context, actor ticketing.Actor, eventID string) (ticketing.EventSummary, error)
	ArchiveEvent(ctx context.Context, actor ticketing.Actor, eventID string) (ticketing.EventSummary, error)
	ListEvents(ctx context.Context, actor ticketing.Actor, employeeID string) ([]ticketing.EventSummary, error)
}

type EventAssetService interface {
	SaveEventPoster(ctx context.Context, actor ticketing.Actor, eventID string, input ticketing.EventAssetInput) (ticketing.EventAsset, error)
	GetEventPoster(ctx context.Context, actor ticketing.Actor, eventID string) (ticketing.EventAsset, error)
}

type EligibilityService interface {
	CheckEligibility(ctx context.Context, actor ticketing.Actor, eventID string, employeeID string) (ticketing.EligibilityDecision, error)
	PreviewEligibility(ctx context.Context, actor ticketing.Actor, eventID string, req ticketing.EligibilityPreviewRequest) (ticketing.EligibilityPreviewResponse, error)
	UpdateEligibility(ctx context.Context, actor ticketing.Actor, eventID string, req ticketing.UpdateEligibilityRequest) (ticketing.EligibilityPreviewResponse, error)
	EligibilityImpactReviews(ctx context.Context, actor ticketing.Actor) ([]ticketing.EligibilityImpactReview, error)
	ResolveEligibilityImpactReview(ctx context.Context, actor ticketing.Actor, reviewID string, req ticketing.ResolveImpactReviewRequest) (ticketing.EligibilityImpactReview, error)
}

type RegistrationService interface {
	Book(ctx context.Context, actor ticketing.Actor, eventID string, req ticketing.BookingRequest) (ticketing.BookingResponse, error)
	ListRegistrations(ctx context.Context, actor ticketing.Actor, eventID string) ([]ticketing.RegistrationDetail, error)
	CancelRegistration(ctx context.Context, actor ticketing.Actor, eventID string, registrationID string, req ticketing.CancelRegistrationRequest) (ticketing.BookingResponse, error)
	CancelMyRegistration(ctx context.Context, actor ticketing.Actor, registrationID string, req ticketing.CancelRegistrationRequest) (ticketing.BookingResponse, error)
	PromoteWaitlist(ctx context.Context, actor ticketing.Actor, eventID string) (ticketing.PromoteWaitlistResponse, error)
	RunLottery(ctx context.Context, actor ticketing.Actor, eventID string, req ticketing.LotteryRunRequest) (ticketing.LotteryRun, error)
	LiftBookingBan(ctx context.Context, actor ticketing.Actor, eventID, employeeID string) error
}

type TicketService interface {
	ListTickets(ctx context.Context, actor ticketing.Actor, employeeID string) ([]ticketing.Ticket, error)
	GetTicket(ctx context.Context, actor ticketing.Actor, ticketID string) (ticketing.Ticket, error)
	RevokeTicket(ctx context.Context, actor ticketing.Actor, ticketID string, req ticketing.RevokeTicketRequest) (ticketing.Ticket, error)
}

type CheckinService interface {
	CheckIn(ctx context.Context, actor ticketing.Actor, req ticketing.CheckinRequest) (ticketing.CheckinResponse, error)
	OfflineCheckinPackage(ctx context.Context, actor ticketing.Actor, eventID string, deviceID string) (ticketing.OfflineCheckinPackage, error)
	SyncOfflineCheckins(ctx context.Context, actor ticketing.Actor, req ticketing.OfflineCheckinSyncRequest) (ticketing.OfflineCheckinSyncResponse, error)
}

type NotificationService interface {
	GetNotificationPreferences(ctx context.Context, actor ticketing.Actor) (ticketing.NotificationPreferences, error)
	UpdateNotificationPreferences(ctx context.Context, actor ticketing.Actor, req ticketing.NotificationPreferences) (ticketing.NotificationPreferences, error)
	NotificationDeliveries(ctx context.Context, actor ticketing.Actor) ([]ticketing.NotificationDelivery, error)
	RetryNotificationDelivery(ctx context.Context, actor ticketing.Actor, deliveryID string) (ticketing.NotificationDelivery, error)
	NotificationDeliveryOpsFeed(ctx context.Context, actor ticketing.Actor, query ...ticketing.NotificationDeliveryOpsQuery) (ticketing.NotificationDeliveryOpsPage, error)
}

type ReportingService interface {
	Reports(ctx context.Context, actor ticketing.Actor) (ticketing.ReportsResult, error)
	CreateReportExport(ctx context.Context, actor ticketing.Actor, req ticketing.ReportExportRequest) (ticketing.ReportExport, error)
	GetReportExport(ctx context.Context, actor ticketing.Actor, exportID string) (ticketing.ReportExport, error)
}

type AuditService interface {
	AuditLogs(ctx context.Context, actor ticketing.Actor, query ...ticketing.AuditLogQuery) ([]ticketing.AuditLog, error)
}

type HRMetadataService interface {
	AdminHROptions(ctx context.Context, actor ticketing.Actor) (ticketing.AdminHROptions, error)
}

type OpsService interface {
	OutboxQueueStatus(ctx context.Context, actor ticketing.Actor) (ticketing.OutboxQueueStatus, error)
	CapacityPressure(ctx context.Context, actor ticketing.Actor) (ticketing.CapacityPressure, error)
	ReportFreshness(ctx context.Context, actor ticketing.Actor, thresholdSeconds int) (ticketing.ReportFreshness, error)
	OpsDashboard(ctx context.Context, actor ticketing.Actor, thresholdSeconds int) (ticketing.OpsDashboard, error)
}

type DemoService interface {
	SeedDemoData(ctx context.Context) error
}
