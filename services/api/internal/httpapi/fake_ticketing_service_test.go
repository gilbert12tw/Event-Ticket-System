package httpapi

import (
	"context"

	"event-ticket-system/internal/ticketing"
	"event-ticket-system/internal/traceid"
)

type fakeTicketingService struct {
	createActor ticketing.Actor
	createTrace string
	bookActor   ticketing.Actor
	checkinErr  error
	auditQuery  []ticketing.AuditLogQuery
}

func (s *fakeTicketingService) CreateEvent(ctx context.Context, actor ticketing.Actor, _ ticketing.CreateEventRequest) (ticketing.EventSummary, error) {
	s.createActor = actor
	s.createTrace = traceid.FromContext(ctx)
	return ticketing.EventSummary{Event: ticketing.Event{EventID: "evt_1", Title: "Demo"}}, nil
}

func (s *fakeTicketingService) ListAdminEvents(context.Context, ticketing.Actor) ([]ticketing.EventSummary, error) {
	return []ticketing.EventSummary{{Event: ticketing.Event{EventID: "evt_1", Title: "Demo"}}}, nil
}

func (s *fakeTicketingService) GetEvent(context.Context, ticketing.Actor, string, string) (ticketing.EventSummary, error) {
	return ticketing.EventSummary{Event: ticketing.Event{EventID: "evt_1", Title: "Demo"}}, nil
}

func (s *fakeTicketingService) UpdateEvent(context.Context, ticketing.Actor, string, ticketing.UpdateEventRequest) (ticketing.EventSummary, error) {
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

func (s *fakeTicketingService) ListEvents(context.Context, ticketing.Actor, string) ([]ticketing.EventSummary, error) {
	return []ticketing.EventSummary{{Event: ticketing.Event{EventID: "evt_1", Title: "Demo"}}}, nil
}

func (s *fakeTicketingService) CheckEligibility(context.Context, ticketing.Actor, string, string) (map[string]interface{}, error) {
	return map[string]interface{}{"eligible": true}, nil
}

func (s *fakeTicketingService) PreviewEligibility(context.Context, ticketing.Actor, string, ticketing.EligibilityPreviewRequest) (ticketing.EligibilityPreviewResponse, error) {
	return ticketing.EligibilityPreviewResponse{EventID: "evt_1", MatchCount: 1}, nil
}

func (s *fakeTicketingService) UpdateEligibility(context.Context, ticketing.Actor, string, ticketing.UpdateEligibilityRequest) (ticketing.EligibilityPreviewResponse, error) {
	return ticketing.EligibilityPreviewResponse{EventID: "evt_1", MatchCount: 1}, nil
}

func (s *fakeTicketingService) EligibilityImpactReviews(context.Context, ticketing.Actor) ([]ticketing.EligibilityImpactReview, error) {
	return []ticketing.EligibilityImpactReview{{ReviewID: "rev_1"}}, nil
}

func (s *fakeTicketingService) ResolveEligibilityImpactReview(context.Context, ticketing.Actor, string, ticketing.ResolveImpactReviewRequest) (ticketing.EligibilityImpactReview, error) {
	return ticketing.EligibilityImpactReview{ReviewID: "rev_1", Status: "resolved"}, nil
}

func (s *fakeTicketingService) Book(_ context.Context, actor ticketing.Actor, _ string, _ ticketing.BookingRequest) (ticketing.BookingResponse, error) {
	s.bookActor = actor
	return ticketing.BookingResponse{Registration: ticketing.Registration{RegistrationID: "reg_1"}}, nil
}

func (s *fakeTicketingService) ListRegistrations(context.Context, ticketing.Actor, string) ([]ticketing.RegistrationDetail, error) {
	return []ticketing.RegistrationDetail{{Registration: ticketing.Registration{RegistrationID: "reg_1"}}}, nil
}

func (s *fakeTicketingService) CancelRegistration(context.Context, ticketing.Actor, string, string, ticketing.CancelRegistrationRequest) (ticketing.BookingResponse, error) {
	return ticketing.BookingResponse{Registration: ticketing.Registration{RegistrationID: "reg_1", Status: ticketing.RegistrationCancelled}}, nil
}

func (s *fakeTicketingService) PromoteWaitlist(context.Context, ticketing.Actor, string) (ticketing.PromoteWaitlistResponse, error) {
	return ticketing.PromoteWaitlistResponse{Message: "promoted"}, nil
}

func (s *fakeTicketingService) RunLottery(context.Context, ticketing.Actor, string, ticketing.LotteryRunRequest) (ticketing.LotteryRun, error) {
	return ticketing.LotteryRun{RunID: "lot_1", Status: "completed"}, nil
}

func (s *fakeTicketingService) ListTickets(context.Context, ticketing.Actor, string) ([]ticketing.Ticket, error) {
	return []ticketing.Ticket{{TicketID: "tkt_1"}}, nil
}

func (s *fakeTicketingService) GetTicket(context.Context, ticketing.Actor, string) (ticketing.Ticket, error) {
	return ticketing.Ticket{TicketID: "tkt_1"}, nil
}

func (s *fakeTicketingService) RevokeTicket(context.Context, ticketing.Actor, string, ticketing.RevokeTicketRequest) (ticketing.Ticket, error) {
	return ticketing.Ticket{TicketID: "tkt_1", Status: ticketing.TicketRevoked}, nil
}

func (s *fakeTicketingService) CheckIn(context.Context, ticketing.Actor, ticketing.CheckinRequest) (ticketing.CheckinResponse, error) {
	return ticketing.CheckinResponse{CheckinID: "chk_1", Duplicate: s.checkinErr != nil}, s.checkinErr
}

func (s *fakeTicketingService) OfflineCheckinPackage(context.Context, ticketing.Actor, string, string) (ticketing.OfflineCheckinPackage, error) {
	return ticketing.OfflineCheckinPackage{BatchID: "off_1", EventID: "evt_1", PackageSignature: "sig_1", TicketCount: 1}, nil
}

func (s *fakeTicketingService) SyncOfflineCheckins(context.Context, ticketing.Actor, ticketing.OfflineCheckinSyncRequest) (ticketing.OfflineCheckinSyncResponse, error) {
	return ticketing.OfflineCheckinSyncResponse{BatchID: "off_1", Accepted: 1}, nil
}

func (s *fakeTicketingService) GetNotificationPreferences(context.Context, ticketing.Actor) (ticketing.NotificationPreferences, error) {
	return ticketing.NotificationPreferences{EmployeeID: "E1001", EmailEnabled: true, InAppEnabled: true}, nil
}

func (s *fakeTicketingService) UpdateNotificationPreferences(context.Context, ticketing.Actor, ticketing.NotificationPreferences) (ticketing.NotificationPreferences, error) {
	return ticketing.NotificationPreferences{EmployeeID: "E1001", EmailEnabled: true, InAppEnabled: true}, nil
}

func (s *fakeTicketingService) NotificationDeliveries(context.Context, ticketing.Actor) ([]ticketing.NotificationDelivery, error) {
	return []ticketing.NotificationDelivery{{DeliveryID: "del_1"}}, nil
}

func (s *fakeTicketingService) RetryNotificationDelivery(context.Context, ticketing.Actor, string) (ticketing.NotificationDelivery, error) {
	return ticketing.NotificationDelivery{DeliveryID: "del_1", Status: "pending"}, nil
}

func (s *fakeTicketingService) Reports(context.Context, ticketing.Actor) ([]ticketing.ReportRow, error) {
	return []ticketing.ReportRow{{EventID: "evt_1"}}, nil
}

func (s *fakeTicketingService) CreateReportExport(context.Context, ticketing.Actor, ticketing.ReportExportRequest) (ticketing.ReportExport, error) {
	return ticketing.ReportExport{ExportID: "exp_1", Status: "ready"}, nil
}

func (s *fakeTicketingService) GetReportExport(context.Context, ticketing.Actor, string) (ticketing.ReportExport, error) {
	return ticketing.ReportExport{ExportID: "exp_1", Status: "ready", ObjectKey: "exports/exp_1.csv"}, nil
}

func (s *fakeTicketingService) AuditLogs(_ context.Context, _ ticketing.Actor, query ...ticketing.AuditLogQuery) ([]ticketing.AuditLog, error) {
	s.auditQuery = query
	return []ticketing.AuditLog{{AuditID: "aud_1"}}, nil
}

func (s *fakeTicketingService) SeedDemoData(context.Context) error {
	return nil
}
