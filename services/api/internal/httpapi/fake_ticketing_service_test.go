package httpapi

import (
	"context"

	"event-ticket-system/internal/ticketing"
	"event-ticket-system/internal/traceid"
)

type fakeTicketingService struct {
	createActor           ticketing.Actor
	createTrace           string
	listEventsActor       ticketing.Actor
	listEventsEmployeeID  string
	getEventEmployeeID    string
	eligibilityActor      ticketing.Actor
	eligibilityEmployeeID string
	bookActor             ticketing.Actor
	bookCalled            bool
	bookRequest           ticketing.BookingRequest
	cancelMyActor         ticketing.Actor
	cancelMyRegistration  string
	cancelMyRequest       ticketing.CancelRegistrationRequest
	listTicketsActor      ticketing.Actor
	listTicketsEmployeeID string
	checkinActor          ticketing.Actor
	checkinErr            error
	offlineSyncActor      ticketing.Actor
	reportsActor          ticketing.Actor
	auditActor            ticketing.Actor
	auditQuery            []ticketing.AuditLogQuery
	createRequest         ticketing.CreateEventRequest
	updateRequest         ticketing.UpdateEventRequest
}

func (s *fakeTicketingService) CreateEvent(ctx context.Context, actor ticketing.Actor, req ticketing.CreateEventRequest) (ticketing.EventSummary, error) {
	s.createActor = actor
	s.createTrace = traceid.FromContext(ctx)
	s.createRequest = req
	return ticketing.EventSummary{Event: ticketing.Event{EventID: "evt_1", Title: "Demo"}}, nil
}

func (s *fakeTicketingService) ListAdminEvents(context.Context, ticketing.Actor) ([]ticketing.EventSummary, error) {
	return []ticketing.EventSummary{{Event: ticketing.Event{EventID: "evt_1", Title: "Demo"}}}, nil
}

func (s *fakeTicketingService) GetEvent(_ context.Context, _ ticketing.Actor, _ string, employeeID string) (ticketing.EventSummary, error) {
	s.getEventEmployeeID = employeeID
	return ticketing.EventSummary{Event: ticketing.Event{EventID: "evt_1", Title: "Demo"}}, nil
}

func (s *fakeTicketingService) UpdateEvent(_ context.Context, _ ticketing.Actor, _ string, req ticketing.UpdateEventRequest) (ticketing.EventSummary, error) {
	s.updateRequest = req
	return ticketing.EventSummary{Event: ticketing.Event{EventID: "evt_1", Title: "Updated", Version: 2}}, nil
}

func (s *fakeTicketingService) ChangeEventState(context.Context, ticketing.Actor, string, ticketing.ChangeEventStateRequest) (ticketing.EventSummary, error) {
	return ticketing.EventSummary{Event: ticketing.Event{EventID: "evt_1", Status: ticketing.EventStatusClosed}}, nil
}

func (s *fakeTicketingService) DuplicateEvent(context.Context, ticketing.Actor, string) (ticketing.EventSummary, error) {
	return ticketing.EventSummary{Event: ticketing.Event{EventID: "evt_copy", Status: ticketing.EventStatusDraft}}, nil
}

func (s *fakeTicketingService) ArchiveEvent(context.Context, ticketing.Actor, string) (ticketing.EventSummary, error) {
	return ticketing.EventSummary{Event: ticketing.Event{EventID: "evt_1", Status: ticketing.EventStatusArchived}}, nil
}

func (s *fakeTicketingService) ListEvents(_ context.Context, actor ticketing.Actor, employeeID string) ([]ticketing.EventSummary, error) {
	s.listEventsActor = actor
	s.listEventsEmployeeID = employeeID
	return []ticketing.EventSummary{{Event: ticketing.Event{EventID: "evt_1", Title: "Demo"}}}, nil
}

func (s *fakeTicketingService) CheckEligibility(_ context.Context, actor ticketing.Actor, _ string, employeeID string) (ticketing.EligibilityDecision, error) {
	s.eligibilityActor = actor
	s.eligibilityEmployeeID = employeeID
	return ticketing.EligibilityDecision{Eligible: true}, nil
}

func (s *fakeTicketingService) PreviewEligibility(context.Context, ticketing.Actor, string, ticketing.EligibilityPreviewRequest) (ticketing.EligibilityPreviewResponse, error) {
	return ticketing.EligibilityPreviewResponse{EventID: "evt_1", MatchCount: 1}, nil
}

func (s *fakeTicketingService) UpdateEligibility(context.Context, ticketing.Actor, string, ticketing.UpdateEligibilityRequest) (ticketing.EligibilityPreviewResponse, error) {
	return ticketing.EligibilityPreviewResponse{EventID: "evt_1", MatchCount: 1}, nil
}

func (s *fakeTicketingService) EligibilityImpactReviews(context.Context, ticketing.Actor) ([]ticketing.EligibilityImpactReview, error) {
	return []ticketing.EligibilityImpactReview{{ReviewID: "rev_1", EmployeeRef: "E100****"}}, nil
}

func (s *fakeTicketingService) ResolveEligibilityImpactReview(context.Context, ticketing.Actor, string, ticketing.ResolveImpactReviewRequest) (ticketing.EligibilityImpactReview, error) {
	return ticketing.EligibilityImpactReview{ReviewID: "rev_1", EmployeeRef: "E100****", Status: "resolved"}, nil
}

func (s *fakeTicketingService) Book(_ context.Context, actor ticketing.Actor, _ string, req ticketing.BookingRequest) (ticketing.BookingResponse, error) {
	s.bookActor = actor
	s.bookCalled = true
	s.bookRequest = req
	return ticketing.BookingResponse{Registration: ticketing.Registration{RegistrationID: "reg_1"}}, nil
}

func (s *fakeTicketingService) ListRegistrations(context.Context, ticketing.Actor, string) ([]ticketing.RegistrationDetail, error) {
	return []ticketing.RegistrationDetail{{Registration: ticketing.Registration{RegistrationID: "reg_1"}}}, nil
}

func (s *fakeTicketingService) CancelRegistration(context.Context, ticketing.Actor, string, string, ticketing.CancelRegistrationRequest) (ticketing.BookingResponse, error) {
	return ticketing.BookingResponse{Registration: ticketing.Registration{RegistrationID: "reg_1", Status: ticketing.RegistrationCancelled}}, nil
}

func (s *fakeTicketingService) CancelMyRegistration(_ context.Context, actor ticketing.Actor, registrationID string, req ticketing.CancelRegistrationRequest) (ticketing.BookingResponse, error) {
	s.cancelMyActor = actor
	s.cancelMyRegistration = registrationID
	s.cancelMyRequest = req
	return ticketing.BookingResponse{Registration: ticketing.Registration{RegistrationID: "reg_1", Status: ticketing.RegistrationCancelled}}, nil
}

func (s *fakeTicketingService) PromoteWaitlist(context.Context, ticketing.Actor, string) (ticketing.PromoteWaitlistResponse, error) {
	return ticketing.PromoteWaitlistResponse{Message: "promoted"}, nil
}

func (s *fakeTicketingService) RunLottery(context.Context, ticketing.Actor, string, ticketing.LotteryRunRequest) (ticketing.LotteryRun, error) {
	return ticketing.LotteryRun{RunID: "lot_1", Status: "completed"}, nil
}

func (s *fakeTicketingService) LiftBookingBan(context.Context, ticketing.Actor, string, string) error {
	return nil
}

func (s *fakeTicketingService) ListTickets(_ context.Context, actor ticketing.Actor, employeeID string) ([]ticketing.Ticket, error) {
	s.listTicketsActor = actor
	s.listTicketsEmployeeID = employeeID
	return []ticketing.Ticket{{TicketID: "tkt_1"}}, nil
}

func (s *fakeTicketingService) GetTicket(context.Context, ticketing.Actor, string) (ticketing.Ticket, error) {
	return ticketing.Ticket{TicketID: "tkt_1"}, nil
}

func (s *fakeTicketingService) RevokeTicket(context.Context, ticketing.Actor, string, ticketing.RevokeTicketRequest) (ticketing.Ticket, error) {
	return ticketing.Ticket{TicketID: "tkt_1", Status: ticketing.TicketRevoked}, nil
}

func (s *fakeTicketingService) CheckIn(_ context.Context, actor ticketing.Actor, _ ticketing.CheckinRequest) (ticketing.CheckinResponse, error) {
	s.checkinActor = actor
	return ticketing.CheckinResponse{CheckinID: "chk_1", Duplicate: s.checkinErr != nil}, s.checkinErr
}

func (s *fakeTicketingService) OfflineCheckinPackage(context.Context, ticketing.Actor, string, string) (ticketing.OfflineCheckinPackage, error) {
	return ticketing.OfflineCheckinPackage{BatchID: "off_1", EventID: "evt_1", PackageSignature: "sig_1", TicketCount: 1}, nil
}

func (s *fakeTicketingService) SyncOfflineCheckins(_ context.Context, actor ticketing.Actor, _ ticketing.OfflineCheckinSyncRequest) (ticketing.OfflineCheckinSyncResponse, error) {
	s.offlineSyncActor = actor
	return ticketing.OfflineCheckinSyncResponse{BatchID: "off_1", Accepted: 1}, nil
}

func (s *fakeTicketingService) GetNotificationPreferences(context.Context, ticketing.Actor) (ticketing.NotificationPreferences, error) {
	return ticketing.NotificationPreferences{EmployeeID: "E1001", EmailEnabled: true, InAppEnabled: true}, nil
}

func (s *fakeTicketingService) UpdateNotificationPreferences(context.Context, ticketing.Actor, ticketing.NotificationPreferences) (ticketing.NotificationPreferences, error) {
	return ticketing.NotificationPreferences{EmployeeID: "E1001", EmailEnabled: true, InAppEnabled: true}, nil
}

func (s *fakeTicketingService) NotificationDeliveries(context.Context, ticketing.Actor) ([]ticketing.NotificationDelivery, error) {
	return []ticketing.NotificationDelivery{{DeliveryID: "del_1", EmployeeRef: "E100****"}}, nil
}

func (s *fakeTicketingService) RetryNotificationDelivery(context.Context, ticketing.Actor, string) (ticketing.NotificationDelivery, error) {
	return ticketing.NotificationDelivery{DeliveryID: "del_1", Status: "pending"}, nil
}

func (s *fakeTicketingService) Reports(_ context.Context, actor ticketing.Actor) (ticketing.ReportsResult, error) {
	s.reportsActor = actor
	return ticketing.ReportsResult{Rows: []ticketing.ReportRow{{EventID: "evt_1"}}}, nil
}

func (s *fakeTicketingService) CreateReportExport(context.Context, ticketing.Actor, ticketing.ReportExportRequest) (ticketing.ReportExport, error) {
	return ticketing.ReportExport{ExportID: "exp_1", ReportType: ticketing.ReportExportTypeParticipation, Format: ticketing.ReportExportFormatCSV, Status: "ready"}, nil
}

func (s *fakeTicketingService) GetReportExport(context.Context, ticketing.Actor, string) (ticketing.ReportExport, error) {
	return ticketing.ReportExport{ExportID: "exp_1", ReportType: ticketing.ReportExportTypeParticipation, Format: ticketing.ReportExportFormatCSV, Status: "ready", ObjectKey: "exports/exp_1.csv"}, nil
}

func (s *fakeTicketingService) AuditLogs(_ context.Context, actor ticketing.Actor, query ...ticketing.AuditLogQuery) ([]ticketing.AuditLog, error) {
	s.auditActor = actor
	s.auditQuery = query
	return []ticketing.AuditLog{{AuditID: "aud_1"}}, nil
}

func (s *fakeTicketingService) SeedDemoData(context.Context) error {
	return nil
}
