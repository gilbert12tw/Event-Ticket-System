package ticketing

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventGovernancePersistsVersionsAuditsAndRejectsIllegalRollback(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Governance Event",
		Capacity: 10,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM event_versions WHERE event_id = $1`, event.EventID, 1)

	nextTitle := "Governance Event Updated"
	nextCapacity := 12
	updated, err := service.UpdateEvent(ctx, admin, event.EventID, UpdateEventRequest{
		Title:    &nextTitle,
		Capacity: &nextCapacity,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Version)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM event_versions WHERE event_id = $1`, event.EventID, 2)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'event.updated' AND entity_id = $1`, event.EventID, 1)

	_, err = service.ChangeEventState(ctx, admin, event.EventID, ChangeEventStateRequest{Status: EventStatusDraft})
	require.Error(t, err, "expected illegal rollback conflict")
	assert.Equal(t, 409, ErrorStatus(err))
	assertRowCount(t, service, ctx, `SELECT count(*) FROM event_versions WHERE event_id = $1`, event.EventID, 2)

	closed, err := service.ChangeEventState(ctx, admin, event.EventID, ChangeEventStateRequest{Status: EventStatusClosed, Reason: "finished"})
	require.NoError(t, err)
	assert.Equal(t, EventStatusClosed, closed.Status)
	assert.Equal(t, 3, closed.Version)
	archived, err := service.ArchiveEvent(ctx, admin, event.EventID)
	require.NoError(t, err)
	assert.Equal(t, EventStatusArchived, archived.Status)
	assert.False(t, archived.ArchivedAt.IsZero())
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'event.state_changed' AND entity_id = $1`, event.EventID, 2)

	duplicate, err := service.DuplicateEvent(ctx, admin, event.EventID)
	require.NoError(t, err)
	assert.Equal(t, EventStatusDraft, duplicate.Status)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM event_versions WHERE event_id = $1`, duplicate.EventID, 1)
}

func TestEventCapacityTypesValidateAndExposeSummaries(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	_, err := service.CreateEvent(ctx, admin, CreateEventRequest{Title: "Missing Capacity"})
	require.Error(t, err, "missing limited capacity should error")
	assert.Equal(t, 400, ErrorStatus(err))
	_, err = service.CreateEvent(ctx, admin, CreateEventRequest{Title: "Zero Capacity", Capacity: 0})
	require.Error(t, err, "zero limited capacity should error")
	assert.Equal(t, 400, ErrorStatus(err))
	_, err = service.CreateEvent(ctx, admin, CreateEventRequest{Title: "Family Limited", Capacity: 10, AllowsFamily: true})
	require.Error(t, err, "limited family should error")
	assert.Equal(t, 400, ErrorStatus(err))

	legacy, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Legacy Limited",
		Location: "Taipei HQ",
		Capacity: 2,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	assert.Equal(t, CapacityTypeLimited, legacy.CapacityType)
	assert.Equal(t, 2, capacityValue(legacy.Capacity))
	require.NotNil(t, legacy.RemainingCapacity)
	assert.Equal(t, 2, *legacy.RemainingCapacity)
	assert.Equal(t, "Taipei", legacy.EventCity)
	assert.Equal(t, "Taipei HQ", legacy.EventSite)
	assert.False(t, legacy.AllowsFamily)

	unlimited, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:        "Unlimited Family",
		EventCity:    "Taipei HQ",
		EventSite:    "HQ",
		CapacityType: CapacityTypeUnlimited,
		AllowsFamily: true,
		Status:       EventStatusPublished,
		Rule:         RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	assert.Equal(t, CapacityTypeUnlimited, unlimited.CapacityType)
	assert.Nil(t, unlimited.Capacity)
	assert.Nil(t, unlimited.RemainingCapacity)
	assert.True(t, unlimited.AllowsFamily)
	assert.Equal(t, "Taipei", unlimited.EventCity)
	assert.Equal(t, "HQ", unlimited.EventSite)
	unlimitedBooking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, unlimited.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "unlimited-booking", FamilyCount: 2})
	require.NoError(t, err)
	assert.Equal(t, RegistrationConfirmed, unlimitedBooking.Registration.Status)
	assert.Equal(t, 2, unlimitedBooking.Registration.FamilyCount)
	assert.NotNil(t, unlimitedBooking.Ticket)

	limitedType := CapacityTypeLimited
	_, err = service.UpdateEvent(ctx, admin, unlimited.EventID, UpdateEventRequest{CapacityType: &limitedType})
	require.Error(t, err, "invalid update to limited without capacity should error")
	assert.Equal(t, 400, ErrorStatus(err))
	familyLimitedCapacity := 3
	disallowFamily := false
	_, err = service.UpdateEvent(ctx, admin, unlimited.EventID, UpdateEventRequest{CapacityType: &limitedType, Capacity: &familyLimitedCapacity, AllowsFamily: &disallowFamily})
	require.Error(t, err, "unlimited event with family registrations should not become limited")
	assert.Equal(t, 409, ErrorStatus(err))
	allowFamily := true
	_, err = service.UpdateEvent(ctx, admin, legacy.EventID, UpdateEventRequest{AllowsFamily: &allowFamily})
	require.Error(t, err, "invalid limited family update should error")
	assert.Equal(t, 400, ErrorStatus(err))

	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, legacy.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "capacity-type-1"})
	require.NoError(t, err)
	_, err = service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, legacy.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "capacity-type-2"})
	require.NoError(t, err)
	one := 1
	_, err = service.UpdateEvent(ctx, admin, legacy.EventID, UpdateEventRequest{Capacity: &one})
	require.Error(t, err, "capacity below confirmed should error")
	assert.Equal(t, 409, ErrorStatus(err))
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
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	events, err := service.ListAdminEvents(ctx, staff)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, event.EventID, events[0].EventID)

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
		require.Error(t, err, "staff %s should error", name)
		assert.Equal(t, 403, ErrorStatus(err), "staff %s", name)
	}
}

func TestProductionRBACSystemAdminAliasesHRAdmin(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	system := Actor{ID: "system-1", Role: RoleSystemAdmin}
	_, err := service.Reports(ctx, system)
	require.NoError(t, err, "system admin should read reports")
	_, err = service.AuditLogs(ctx, system)
	require.NoError(t, err, "system admin should read audit logs")
	_, err = service.Book(ctx, system, "evt", BookingRequest{EmployeeID: "E1001", IdempotencyKey: "system-book"})
	require.Error(t, err, "system admin must not book as employee")
	assert.Equal(t, 403, ErrorStatus(err))
}

func TestProviderRoleRBACMatrix(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	event, err := service.CreateEvent(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:    "Provider RBAC Matrix",
		Capacity: 10,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

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
				_, err := service.CheckIn(ctx, actor, CheckinRequest{SignedToken: "invalid.token", EventID: event.EventID, DeviceID: "gate-rbac"})
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
				assert.Equal(t, wantStatus, gotStatus, "err = %v", err)
			})
		}
	}
}

func TestEligibilityPreviewRequiresExistingEvent(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	_, err := service.PreviewEligibility(ctx, Actor{ID: "admin-1", Role: RoleActivityAdmin}, "missing-event", EligibilityPreviewRequest{
		Rule: RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.Error(t, err, "expected missing event 404")
	assert.Equal(t, 404, ErrorStatus(err))
}

func TestInvalidCheckinAttemptsAreAuditedWithoutTokenLeak(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()

	_, err := service.CheckIn(ctx, Actor{ID: "staff-1", Role: RoleCheckinStaff}, CheckinRequest{SignedToken: "bad.token.secret", EventID: "evt-invalid", DeviceID: "gate-1"})
	require.Error(t, err, "expected bad token 400")
	assert.Equal(t, 400, ErrorStatus(err))
	var metadata string
	require.NoError(t, service.db.QueryRow(ctx, `SELECT metadata::text FROM audit_logs WHERE action = 'checkin.rejected'`).Scan(&metadata))
	require.NotEmpty(t, metadata, "invalid check-in audit metadata missing")
	assert.False(t, containsAny(metadata, "bad.token.secret", "signed_token"), "invalid check-in audit leaked token metadata: %s", metadata)
}

func TestManualWaitlistPromotionNoopIsAudited(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "No Waitlist",
		Capacity: 5,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	result, err := service.PromoteWaitlist(ctx, admin, event.EventID)
	require.NoError(t, err)
	assert.Equal(t, "no eligible waitlist registrations", result.Message)
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
