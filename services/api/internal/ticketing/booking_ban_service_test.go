package ticketing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookingBanCreatedAfterConfirmedCancellation(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Ban Target",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	employee := Actor{ID: "E1001", Role: RoleEmployee}
	booking, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "ban-book"})
	require.NoError(t, err)
	require.Equal(t, RegistrationConfirmed, booking.Registration.Status)

	_, err = service.CancelMyRegistration(ctx, employee, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "ban-cancel", Reason: "sick"})
	require.NoError(t, err)

	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1 AND lifted_at IS NULL`, "E1001", 1)
}

func TestBookingBanNotCreatedAfterWaitlistCancellation(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Waitlist Ban Target",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	_, err = service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "w-book-1"})
	require.NoError(t, err)

	employee2 := Actor{ID: "E1002", Role: RoleEmployee}
	booking2, err := service.Book(ctx, employee2, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "w-book-2"})
	require.NoError(t, err)
	require.Equal(t, RegistrationWaitlisted, booking2.Registration.Status)

	_, err = service.CancelMyRegistration(ctx, employee2, booking2.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "w-cancel", Reason: "sick"})
	require.NoError(t, err)

	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1 AND lifted_at IS NULL`, "E1002", 0)
}

func TestBannedEmployeeCannotRebook(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Banned Rebook",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	employee := Actor{ID: "E1001", Role: RoleEmployee}
	booking, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "b-book"})
	require.NoError(t, err)

	_, err = service.CancelMyRegistration(ctx, employee, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "b-cancel", Reason: "sick"})
	require.NoError(t, err)

	_, err = service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "b-rebook"})
	require.Error(t, err)
	assert.Equal(t, 422, ErrorStatus(err))
	assert.Equal(t, "BOOKING_BANNED", ErrorCode(err))
}

func TestBannedEmployeeCannotJoinWaitlist(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Banned Waitlist",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	employee := Actor{ID: "E1001", Role: RoleEmployee}
	booking, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "bw-book"})
	require.NoError(t, err)

	_, err = service.CancelMyRegistration(ctx, employee, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "bw-cancel", Reason: "sick"})
	require.NoError(t, err)

	// Another employee takes the spo
	_, err = service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "bw-book-2"})
	require.NoError(t, err)

	// Banned employee tries to book (which would normally waitlist)
	_, err = service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "bw-rebook"})
	require.Error(t, err)
	assert.Equal(t, 422, ErrorStatus(err))
	assert.Equal(t, "BOOKING_BANNED", ErrorCode(err))
}

func TestLiftBanAllowsRebook(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Lift Ban",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	employee := Actor{ID: "E1001", Role: RoleEmployee}
	booking, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lb-book"})
	require.NoError(t, err)

	_, err = service.CancelMyRegistration(ctx, employee, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "lb-cancel", Reason: "sick"})
	require.NoError(t, err)

	err = service.LiftBookingBan(ctx, admin, event.EventID, "E1001")
	require.NoError(t, err)

	// Verify ban is lifted
	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1 AND lifted_at IS NOT NULL`, "E1001", 1)

	rebook, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lb-rebook"})
	require.NoError(t, err)
	require.Equal(t, RegistrationConfirmed, rebook.Registration.Status)
	require.NotNil(t, rebook.Ticket)
	assert.False(t, rebook.Duplicate)
	assert.NotEqual(t, booking.Registration.RegistrationID, rebook.Registration.RegistrationID)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND employee_id = 'E1001'`, event.EventID, 2)
}

func TestLiftBanNotFoundReturns404(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	err := service.LiftBookingBan(ctx, admin, "non-existent-event", "E1001")
	require.Error(t, err)
	assert.Equal(t, 404, ErrorStatus(err))
}

func TestBanCreationIdempotent(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Idempotent Ban",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	employee := Actor{ID: "E1001", Role: RoleEmployee}
	booking, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "ib-book"})
	require.NoError(t, err)

	_, err = service.CancelMyRegistration(ctx, employee, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "ib-cancel", Reason: "sick"})
	require.NoError(t, err)

	// Call again with same idempotency key
	_, err = service.CancelMyRegistration(ctx, employee, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "ib-cancel", Reason: "sick"})
	require.NoError(t, err)

	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1`, "E1001", 1)
}

func TestPhase1DataNotRetroactivelyBanned(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Phase1 Data",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	// Manually insert a cancelled registration with no ban row
	_, err = service.db.Exec(ctx, `INSERT INTO registrations (registration_id, event_id, employee_id, status, idempotency_key, cancel_idempotency_key, cancelled_at, created_at)
		VALUES ('reg_p1', $1, 'E1001', 'cancelled', 'p1_book', 'p1_cancel', $2, $2)`, event.EventID, now)
	require.NoError(t, err)

	// Booking should succeed
	employee := Actor{ID: "E1001", Role: RoleEmployee}
	_, err = service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "p1-book2"})
	require.NoError(t, err)
}

func TestAuditLogWrittenOnBanAndLift(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Audit Ban",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	employee := Actor{ID: "E1001", Role: RoleEmployee}
	booking, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "ab-book"})
	require.NoError(t, err)

	_, err = service.CancelMyRegistration(ctx, employee, booking.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "ab-cancel", Reason: "sick"})
	require.NoError(t, err)

	// Verify ban created audit log
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = $1 AND entity_type = 'booking_ban'`, "booking.ban_created", 1)

	err = service.LiftBookingBan(ctx, admin, event.EventID, "E1001")
	require.NoError(t, err)

	// Verify ban lifted audit log
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = $1 AND entity_type = 'booking_ban'`, "booking.ban_lifted", 1)
}

func TestLiftBanRebookAndConfirmedCancelCreatesSecondBan(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "Lift And Rebook",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	employee := Actor{ID: "E1001", Role: RoleEmployee}

	// Book → cancel → ban created.
	booking1, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lrb-book-1"})
	require.NoError(t, err)
	_, err = service.CancelMyRegistration(ctx, employee, booking1.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "lrb-cancel-1", Reason: "sick"})
	require.NoError(t, err)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1 AND lifted_at IS NULL`, "E1001", 1)

	// Lift the ban.
	require.NoError(t, service.LiftBookingBan(ctx, admin, event.EventID, "E1001"))
	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1 AND lifted_at IS NULL`, "E1001", 0)

	// Rebook creates a new active registration rather than returning the historical cancelled row.
	rebook, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lrb-book-2"})
	require.NoError(t, err)
	require.Equal(t, RegistrationConfirmed, rebook.Registration.Status)
	require.NotNil(t, rebook.Ticket)
	assert.False(t, rebook.Duplicate)
	assert.NotEqual(t, booking1.Registration.RegistrationID, rebook.Registration.RegistrationID)

	_, err = service.CancelMyRegistration(ctx, employee, rebook.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "lrb-cancel-2", Reason: "sick again"})
	require.NoError(t, err)

	assertRowCount(t, service, ctx, `SELECT count(*) FROM registrations WHERE event_id = $1 AND employee_id = 'E1001'`, event.EventID, 2)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1`, "E1001", 2)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1 AND lifted_at IS NULL`, "E1001", 1)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = $1 AND entity_type = 'booking_ban'`, "booking.ban_created", 2)
}

// TestCreateBookingBanTxInsertsNewRowAfterLiftedBan verifies that the partial unique index
// (WHERE lifted_at IS NULL) allows a second INSERT to succeed after the first ban has been
// lifted. This directly tests the database behaviour that makes re-banning possible via the
// service layer.
func TestCreateBookingBanTxInsertsNewRowAfterLiftedBan(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	require.NoError(t, service.SeedDemoData(ctx))

	now := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:             "DB Re-Ban Test",
		Capacity:          1,
		Status:            EventStatusPublished,
		StartsAt:          now.Add(48 * time.Hour),
		RegistrationStart: now.Add(-time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		Rule:              RuleInput{Department: "Engineering", Site: "Taipei HQ", MinGrade: 5, EmploymentStatus: "active"},
	})
	require.NoError(t, err)

	employee := Actor{ID: "E1001", Role: RoleEmployee}

	// Create the first ban through the normal cancel flow.
	booking1, err := service.Book(ctx, employee, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "drb-book-1"})
	require.NoError(t, err)
	_, err = service.CancelMyRegistration(ctx, employee, booking1.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "drb-cancel-1", Reason: "sick"})
	require.NoError(t, err)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1 AND lifted_at IS NULL`, "E1001", 1)

	// Lift the ban.
	require.NoError(t, service.LiftBookingBan(ctx, admin, event.EventID, "E1001"))
	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1 AND lifted_at IS NULL`, "E1001", 0)

	// Directly call createBookingBanTx to verify the partial index allows a fresh INSERT.
	tx, err := service.db.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	err = service.createBookingBanTx(ctx, tx, admin, event.EventID, "E1001", booking1.Registration.RegistrationID, "second cancel")
	require.NoError(t, err, "partial index must allow INSERT after lifted ban")
	require.NoError(t, tx.Commit(ctx))

	// Both lifted and new-active rows exist; exactly one is active.
	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1`, "E1001", 2)
	assertRowCount(t, service, ctx, `SELECT count(*) FROM booking_bans WHERE employee_id = $1 AND lifted_at IS NULL`, "E1001", 1)

	// Two ban_created audit entries (one per ban cycle).
	assertRowCount(t, service, ctx, `SELECT count(*) FROM audit_logs WHERE action = $1 AND entity_type = 'booking_ban'`, "booking.ban_created", 2)
}
