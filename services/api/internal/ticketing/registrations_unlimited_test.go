package ticketing

import (
	"context"
	"testing"
)

func TestUnlimitedEventBookingConfirmsAndPersistsFamilyCount(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:        "Open House",
		Location:     "Taipei HQ",
		CapacityType: CapacityTypeUnlimited,
		Status:       EventStatusPublished,
		Rule:         RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !event.AllowsFamily {
		t.Fatalf("unlimited create should auto-enable allows_family, got %+v", event)
	}

	first, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "unl-1", FamilyCount: 3})
	if err != nil {
		t.Fatal(err)
	}
	if first.Registration.Status != RegistrationConfirmed || first.Ticket == nil {
		t.Fatalf("unlimited booking should confirm and issue ticket, got %+v", first)
	}
	if first.Registration.FamilyCount != 3 {
		t.Fatalf("family_count = %d, want 3", first.Registration.FamilyCount)
	}
	if first.RemainingCapacity != 0 {
		t.Fatalf("unlimited remaining_capacity = %d, want 0", first.RemainingCapacity)
	}

	retry, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "unl-1", FamilyCount: 3})
	if err != nil {
		t.Fatal(err)
	}
	if retry.Registration.RegistrationID != first.Registration.RegistrationID || retry.Registration.FamilyCount != 3 {
		t.Fatalf("idempotent retry mismatch: %+v", retry)
	}

	second, err := service.Book(ctx, Actor{ID: "E1002", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1002", IdempotencyKey: "unl-2", FamilyCount: 0})
	if err != nil {
		t.Fatal(err)
	}
	if second.Registration.Status != RegistrationConfirmed {
		t.Fatalf("second unlimited booking status = %s, want confirmed", second.Registration.Status)
	}
}

func TestUnlimitedBookingRejectsFamilyCountOverCap(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:        "Open House",
		Location:     "Taipei HQ",
		CapacityType: CapacityTypeUnlimited,
		Status:       EventStatusPublished,
		Rule:         RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "unl-cap", FamilyCount: 11}); err == nil || ErrorStatus(err) != 400 {
		t.Fatalf("family_count=11 error = %v, want 400", err)
	}
	if _, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "unl-neg", FamilyCount: -1}); err == nil || ErrorStatus(err) != 400 {
		t.Fatalf("family_count=-1 error = %v, want 400", err)
	}
}

func TestLimitedBookingRejectsFamilyCount(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:    "Limited Hike",
		Location: "Taipei HQ",
		Capacity: 5,
		Status:   EventStatusPublished,
		Rule:     RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lim-fam", FamilyCount: 1}); err == nil || ErrorStatus(err) != 400 {
		t.Fatalf("limited+family error = %v, want 400", err)
	}

	ok, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "lim-ok"})
	if err != nil {
		t.Fatal(err)
	}
	if ok.Registration.FamilyCount != 0 || ok.Registration.Status != RegistrationConfirmed {
		t.Fatalf("limited self booking = %+v", ok)
	}
}

func TestUnlimitedBookingCancelAndRebook(t *testing.T) {
	service, cleanup := newIntegrationService(t)
	defer cleanup()
	ctx := context.Background()
	if err := service.SeedDemoData(ctx); err != nil {
		t.Fatal(err)
	}
	admin := Actor{ID: "admin-1", Role: RoleActivityAdmin}
	event, err := service.CreateEvent(ctx, admin, CreateEventRequest{
		Title:        "Open House",
		Location:     "Taipei HQ",
		CapacityType: CapacityTypeUnlimited,
		Status:       EventStatusPublished,
		Rule:         RuleInput{Department: "Engineering", Site: "Taipei", MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	booked, err := service.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{EmployeeID: "E1001", IdempotencyKey: "unl-cancel", FamilyCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CancelRegistration(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, booked.Registration.RegistrationID, CancelRegistrationRequest{IdempotencyKey: "cancel-1", Reason: "change of plans"}); err != nil {
		t.Fatal(err)
	}
}
