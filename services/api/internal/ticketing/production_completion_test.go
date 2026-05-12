package ticketing

import (
	"context"
	"strings"
	"testing"
)

func TestEventGovernancePersistsVersionsAuditsAndRejectsIllegalRollback(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Governance Event",
		Capacity: 10,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM event_versions WHERE event_id = $1`, event.EventID, 1)

	nextTitle := "Governance Event Updated"
	nextCapacity := 12
	updated, err := service.UpdateEvent(ctx, admin, event.EventID, UpdateEventRequest{
		Title:    &nextTitle,
		Capacity: &nextCapacity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 {
		t.Fatalf("updated version = %d, want 2", updated.Version)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM event_versions WHERE event_id = $1`, event.EventID, 2)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'event.updated' AND entity_id = $1`, event.EventID, 1)

	if _, err := service.ChangeEventState(ctx, admin, event.EventID, ChangeEventStateRequest{Status: EventStatusDraft}); err == nil || ErrorStatus(err) != 409 {
		t.Fatalf("expected illegal rollback conflict, got %v", err)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM event_versions WHERE event_id = $1`, event.EventID, 2)

	closed, err := service.ChangeEventState(ctx, admin, event.EventID, ChangeEventStateRequest{Status: EventStatusClosed, Reason: "finished"})
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != EventStatusClosed || closed.Version != 3 {
		t.Fatalf("closed event = %+v, want closed version 3", closed)
	}
	archived, err := service.ArchiveEvent(ctx, admin, event.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.Status != EventStatusArchived || archived.ArchivedAt.IsZero() {
		t.Fatalf("archived event = %+v", archived)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'event.state_changed' AND entity_id = $1`, event.EventID, 2)

	duplicate, err := service.DuplicateEvent(ctx, admin, event.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Status != EventStatusDraft {
		t.Fatalf("duplicate status = %s, want draft", duplicate.Status)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM event_versions WHERE event_id = $1`, duplicate.EventID, 1)
}

func TestCheckinStaffCanListEventsForOfflinePackageSelectionOnly(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	staff := Actor{ID: "staff-1", Role: RoleCheckinStaff}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Offline Package Selection",
		Capacity: 10,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}

	events, err := service.ListAdminEvents(ctx, staff)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID != event.EventID {
		t.Fatalf("staff event list = %+v, want only %s", events, event.EventID)
	}

	nextTitle := "Staff Must Not Edit"
	for name, err := range map[string]error{
		"create": func() error {
			_, err := service.CreateEvent(ctx, staff, CreateEventRequest{Title: nextTitle, Capacity: 1})
			return err
		}(),
		"update": func() error {
			_, err := service.UpdateEvent(ctx, staff, event.EventID, UpdateEventRequest{Title: &nextTitle})
			return err
		}(),
		"state": func() error {
			_, err := service.ChangeEventState(ctx, staff, event.EventID, ChangeEventStateRequest{Status: EventStatusClosed})
			return err
		}(),
		"duplicate": func() error {
			_, err := service.DuplicateEvent(ctx, staff, event.EventID)
			return err
		}(),
		"archive": func() error {
			_, err := service.ArchiveEvent(ctx, staff, event.EventID)
			return err
		}(),
	} {
		if err == nil || ErrorStatus(err) != 403 {
			t.Fatalf("staff %s error = %v, want 403", name, err)
		}
	}
}

func TestProductionRBACSystemAdminAliasesHRAdmin(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	system := Actor{ID: "system-1", Role: RoleSystemAdmin}
	if _, err := service.Reports(ctx, system); err != nil {
		t.Fatalf("system admin should read reports: %v", err)
	}
	if _, err := service.AuditLogs(ctx, system); err != nil {
		t.Fatalf("system admin should read audit logs: %v", err)
	}
	if _, err := service.Book(ctx, system, "evt", BookingRequest{EmployeeID: "E1001", IdempotencyKey: "system-book"}); err == nil || ErrorStatus(err) != 403 {
		t.Fatalf("system admin must not book as employee, got %v", err)
	}
}

func TestProviderRoleRBACMatrix(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Provider RBAC Matrix",
		Capacity: 10,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}

	actors := []Actor{
		{ID: "E1001", Role: RoleEmployee},
		{ID: "admin-1", Role: RoleActivityAdmin},
		{ID: "staff-1", Role: RoleCheckinStaff},
		{ID: "hr-1", Role: RoleHRAdmin},
		{ID: "system-1", Role: RoleSystemAdmin},
	}
	actions := []struct {
		name       string
		allowedFor map[string]int
		run        func(Actor) error
	}{
		{
			name: "book own event",
			allowedFor: map[string]int{
				RoleEmployee: 200,
			},
			run: func(actor Actor) error {
				_, err := service.Book(ctx, actor, event.EventID, BookingRequest{IdempotencyKey: "rbac-book-" + actor.Role})
				return err
			},
		},
		{
			name: "create event",
			allowedFor: map[string]int{
				RoleActivityAdmin: 200,
			},
			run: func(actor Actor) error {
				_, err := service.CreateEvent(ctx, actor, CreateEventRequest{Title: "RBAC Created " + actor.Role, Capacity: 1})
				return err
			},
		},
		{
			name: "check in",
			allowedFor: map[string]int{
				RoleCheckinStaff: 400,
			},
			run: func(actor Actor) error {
				_, err := service.CheckIn(ctx, actor, CheckinRequest{SignedToken: "invalid.token", DeviceID: "gate-rbac"})
				return err
			},
		},
		{
			name: "reports",
			allowedFor: map[string]int{
				RoleHRAdmin:     200,
				RoleSystemAdmin: 200,
			},
			run: func(actor Actor) error {
				_, err := service.Reports(ctx, actor)
				return err
			},
		},
		{
			name: "audit logs",
			allowedFor: map[string]int{
				RoleHRAdmin:     200,
				RoleSystemAdmin: 200,
			},
			run: func(actor Actor) error {
				_, err := service.AuditLogs(ctx, actor)
				return err
			},
		},
	}

	for _, actor := range actors {
		for _, action := range actions {
			t.Run(actor.Role+"/"+action.name, func(t *testing.T) {
				err := action.run(actor)
				gotStatus := 200
				if err != nil {
					gotStatus = ErrorStatus(err)
				}
				wantStatus, allowed := action.allowedFor[actor.Role]
				if !allowed {
					wantStatus = 403
				}
				if gotStatus != wantStatus {
					t.Fatalf("status = %d, want %d, err = %v", gotStatus, wantStatus, err)
				}
			})
		}
	}
}

func TestEligibilityPreviewRequiresExistingEvent(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	_, err := service.PreviewEligibility(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, "missing-event", EligibilityPreviewRequest{
		Rule: RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err == nil || ErrorStatus(err) != 404 {
		t.Fatalf("expected missing event 404, got %v", err)
	}
}

func TestInvalidCheckinAttemptsAreAuditedWithoutTokenLeak(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := service.CheckIn(ctx, Actor{ID: "staff-1", Role: RoleCheckinStaff}, CheckinRequest{SignedToken: "bad.token.secret", DeviceID: "gate-1"})
	if err == nil || ErrorStatus(err) != 400 {
		t.Fatalf("expected bad token 400, got %v", err)
	}
	var metadata string
	if err := service.db.QueryRow(ctx, `SELECT metadata::text FROM audit_logs WHERE action = 'checkin.rejected'`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if metadata == "" || containsAny(metadata, "bad.token.secret", "signed_token") {
		t.Fatalf("invalid check-in audit leaked token metadata: %s", metadata)
	}
}

func TestManualWaitlistPromotionNoopIsAudited(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "No Waitlist",
		Capacity: 5,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.PromoteWaitlist(ctx, admin, event.EventID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Message != "no eligible waitlist registrations" {
		t.Fatalf("promotion message = %q", result.Message)
	}
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'waitlist.promotion_noop' AND entity_id = $1`, event.EventID, 1)
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
