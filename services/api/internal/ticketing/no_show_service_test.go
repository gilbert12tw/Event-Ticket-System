package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessNoShowsAppliesLimitedCooldownAndBlocksLimitedBookings(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	bookingTime := now.Add(-96 * time.Hour)
	service.now = func() time.Time { return bookingTime }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	pastEvent, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Past Limited Event",
		Capacity:          5,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(-48 * time.Hour),
		RegistrationStart: now.Add(-120 * time.Hour),
		RegistrationClose: now.Add(-72 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, pastEvent.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "past-limited-book"})
	if err != nil {
		t.Fatal(err)
	}
	if booking.Ticket == nil {
		t.Fatalf("past booking did not issue ticket: %+v", booking)
	}

	service.now = func() time.Time { return now }
	result, err := service.ProcessNoShows(ctx, Actor{ID: "system-1", Role: RoleSystemAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 1 || result.CooldownsApplied != 1 {
		t.Fatalf("no-show result = %+v", result)
	}

	futureLimited, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Future Limited Event",
		Capacity:          5,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, futureLimited.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "cooldown-limited-book"})
	if err == nil || ErrorStatus(err) != 403 {
		t.Fatalf("cooldown limited booking error = %v, want 403", err)
	}

	unlimited, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Future Unlimited Event",
		CapacityType:      CapacityTypeUnlimited,
		AllowsFamily:      true,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, unlimited.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "cooldown-unlimited-book", FamilyCount: 2})
	if err != nil {
		t.Fatalf("cooldown should not block unlimited booking: %v", err)
	}
}

func TestRecordNoShowSkipsSideEffectsWhenInsertIsDeduplicated(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now.Add(-96 * time.Hour) }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Deduped No Show",
		Capacity:          5,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(-48 * time.Hour),
		RegistrationStart: now.Add(-120 * time.Hour),
		RegistrationClose: now.Add(-72 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "dedupe-no-show-book"})
	require.NoError(t, err)

	service.now = func() time.Time { return now }
	actor := Actor{ID: "system-1", Role: RoleSystemAdmin}
	policy := service.noShowPolicy.Normalize()
	recorded, applied, err := service.recordNoShow(ctx, actor, booking.Registration.RegistrationID, event.EventID, "E1001", policy)
	require.NoError(t, err)
	assert.True(t, recorded)
	assert.True(t, applied)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM no_show_records WHERE registration_id = $1`, booking.Registration.RegistrationID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'registration.no_show_recorded' AND entity_id = $1`, booking.Registration.RegistrationID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'registration.no_show_recorded' AND aggregate_id = $1`, booking.Registration.RegistrationID, 1)

	recorded, applied, err = service.recordNoShow(ctx, actor, booking.Registration.RegistrationID, event.EventID, "E1001", policy)
	require.NoError(t, err)
	assert.False(t, recorded)
	assert.False(t, applied)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM no_show_records WHERE registration_id = $1`, booking.Registration.RegistrationID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = 'registration.no_show_recorded' AND entity_id = $1`, booking.Registration.RegistrationID, 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM outbox_events WHERE event_type = 'registration.no_show_recorded' AND aggregate_id = $1`, booking.Registration.RegistrationID, 1)
}

func TestRecordNoShowWritesSafeGovernanceMetadata(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now.Add(-96 * time.Hour) }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "No Show Governance",
		Capacity:          5,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(-48 * time.Hour),
		RegistrationStart: now.Add(-120 * time.Hour),
		RegistrationClose: now.Add(-72 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "metadata-no-show-book"})
	require.NoError(t, err)

	service.now = func() time.Time { return now }
	actor := Actor{ID: "system-1", Role: RoleSystemAdmin}
	recorded, _, err := service.recordNoShow(ctx, actor, booking.Registration.RegistrationID, event.EventID, "E1001", service.noShowPolicy.Normalize())
	require.NoError(t, err)
	require.True(t, recorded)

	audit := readJSONMap(t, service, ctx, `SELECT metadata::text FROM audit_logs WHERE action = 'registration.no_show_recorded' AND entity_id = $1`, booking.Registration.RegistrationID)
	assert.Equal(t, event.EventID, audit["event_id"])
	assert.Equal(t, "No Show Governance", audit["event_title"])
	assert.Equal(t, booking.Registration.RegistrationID, audit["registration_id"])
	assert.Equal(t, "cooldown_active", audit["cooldown_status"])
	assertNoSensitiveJSONValues(t, audit, "Ariel Chen", booking.Ticket.SignedToken, booking.Ticket.QRPayload)

	payload := readJSONMap(t, service, ctx, `SELECT payload::text FROM outbox_events WHERE event_type = 'registration.no_show_recorded' AND aggregate_id = $1`, booking.Registration.RegistrationID)
	assert.Equal(t, event.EventID, payload["event_id"])
	assert.Equal(t, "No Show Governance", payload["event_title"])
	assert.Equal(t, "E1001", payload["employee_id"])
	assert.Equal(t, "cooldown_active", payload["cooldown_status"])
	assertNoSensitiveJSONValues(t, payload, "Ariel Chen", booking.Ticket.SignedToken, booking.Ticket.QRPayload)
}

func TestEmployeeCancellationCutoffAndAdminReason(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	bookingTime := now.Add(-48 * time.Hour)
	service.now = func() time.Time { return bookingTime }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Closed Cancellation",
		Capacity:          2,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(24 * time.Hour),
		RegistrationStart: now.Add(-72 * time.Hour),
		RegistrationClose: now.Add(-24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	booking, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "closed-cancel-book"})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	_, err = service.CancelMyRegistration(ctx, Actor{ID: "E1001", Role: RoleEmployee}, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "self-cancel-closed", Reason: "sick"})
	if err == nil || ErrorStatus(err) != 409 {
		t.Fatalf("self-cancel cutoff error = %v, want 409", err)
	}
	_, err = service.CancelRegistration(ctx, admin, event.EventID, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "admin-cancel-no-reason"})
	if err == nil || ErrorStatus(err) != 400 {
		t.Fatalf("admin cancel missing reason error = %v, want 400", err)
	}
	cancelled, err := service.CancelRegistration(ctx, admin, event.EventID, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "admin-cancel-reason", Reason: "approved exception"})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Registration.Status != RegistrationCancelled {
		t.Fatalf("admin cancellation = %+v", cancelled)
	}
}
