package ticketing

import (
	"context"
	"fmt"
	"time"
)

const demoSiteTaipeiHQ = "Taipei HQ"

// DemoEmployeeIDs is the ordered list of employee IDs seeded by SeedDemoData.
// Keep in sync with apps/web/src/lib/api/demo-data.ts.
var DemoEmployeeIDs = []string{
	"E1001", "E1002", "E1003",
	"E1004", "E1005", "E1006", "E1007", "E1008", "E1009", "E1010",
}

type DemoEventTicketSeed struct {
	TodayEventID    string
	TodayTicketIDs  []string
	FutureEventID   string
	FutureTicketIDs []string
}

func (s *Service) SeedDemoData(ctx context.Context) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(ctx, tx)
	employees := []Employee{
		{EmployeeID: "E1001", FullName: "Ariel Chen", Department: "Engineering", Site: demoSiteTaipeiHQ, JobGrade: 6, EmploymentStatus: "active"},
		{EmployeeID: "E1002", FullName: "Ben Lin", Department: "Engineering", Site: demoSiteTaipeiHQ, JobGrade: 5, EmploymentStatus: "active"},
		{EmployeeID: "E1003", FullName: "Tina Chang", Department: "Engineering", Site: demoSiteTaipeiHQ, JobGrade: 5, EmploymentStatus: "active"},
		{EmployeeID: "E2001", FullName: "Carla Wu", Department: "Sales", Site: demoSiteTaipeiHQ, JobGrade: 4, EmploymentStatus: "active"},
		{EmployeeID: "E1004", FullName: "David Tan", Department: "Engineering", Site: demoSiteTaipeiHQ, JobGrade: 5, EmploymentStatus: "active"},
		{EmployeeID: "E1005", FullName: "Emily Liu", Department: "Engineering", Site: demoSiteTaipeiHQ, JobGrade: 6, EmploymentStatus: "active"},
		{EmployeeID: "E1006", FullName: "Frank Wang", Department: "Engineering", Site: demoSiteTaipeiHQ, JobGrade: 5, EmploymentStatus: "active"},
		{EmployeeID: "E1007", FullName: "Grace Huang", Department: "Engineering", Site: demoSiteTaipeiHQ, JobGrade: 5, EmploymentStatus: "active"},
		{EmployeeID: "E1008", FullName: "Henry Cheng", Department: "Engineering", Site: demoSiteTaipeiHQ, JobGrade: 5, EmploymentStatus: "active"},
		{EmployeeID: "E1009", FullName: "Iris Yang", Department: "Engineering", Site: demoSiteTaipeiHQ, JobGrade: 6, EmploymentStatus: "active"},
		{EmployeeID: "E1010", FullName: "Jack Kao", Department: "Engineering", Site: demoSiteTaipeiHQ, JobGrade: 5, EmploymentStatus: "active"},
		// E3001 keeps a second distinct site in the demo HR data so admin
		// site filters and eligibility previews stay multi-site. It is not in
		// DemoEmployeeIDs because demo events are Taipei HQ only.
		{EmployeeID: "E3001", FullName: "Tainan User", Department: "Engineering", Site: "Tainan HQ", JobGrade: 5, EmploymentStatus: "active"},
	}
	for _, employee := range employees {
		_, err := tx.Exec(ctx, `INSERT INTO employees (employee_id, full_name, department, site, job_grade, employment_status)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (employee_id) DO UPDATE SET full_name = EXCLUDED.full_name, department = EXCLUDED.department, site = EXCLUDED.site, job_grade = EXCLUDED.job_grade, employment_status = EXCLUDED.employment_status`,
			employee.EmployeeID, employee.FullName, employee.Department, employee.Site, employee.JobGrade, employee.EmploymentStatus)
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) SeedDemoEventTickets(ctx context.Context) (DemoEventTicketSeed, error) {
	// Today event: startsAt = 1 hour in the past so it is always ready for check-in.
	todayStarts := s.now().Add(-1 * time.Hour).UTC().Truncate(time.Second)
	futureStarts := demoNextCheckinStart(demoTodayCheckinStart(s.now()))

	todayEventID, todayTicketIDs, err := s.seedDemoEventTickets(ctx, "Demo Check-in Today", "demo-today", todayStarts, DemoEmployeeIDs)
	if err != nil {
		return DemoEventTicketSeed{}, err
	}
	// Future event: 2 tickets for the demo runbook flow.
	futureEventID, futureTicketIDs, err := s.seedDemoEventTickets(ctx, "Demo Future Check-in", "demo-future", futureStarts, []string{"E1001", "E1002"})
	if err != nil {
		return DemoEventTicketSeed{}, err
	}
	return DemoEventTicketSeed{
		TodayEventID:    todayEventID,
		TodayTicketIDs:  todayTicketIDs,
		FutureEventID:   futureEventID,
		FutureTicketIDs: futureTicketIDs,
	}, nil
}

// seedDemoEventTickets creates one direct-booking event and books every
// employee ID in order. All employees must already exist via SeedDemoData.
func (s *Service) seedDemoEventTickets(ctx context.Context, title string, idempotencyPrefix string, startsAt time.Time, employeeIDs []string) (string, []string, error) {
	now := s.now()
	event, err := s.CreateEvent(ctx, Actor{ID: "demo-admin", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:             title,
		Description:       "Demo activity for check-in readiness validation.",
		Location:          demoSiteTaipeiHQ,
		EventCity:         "Taipei",
		EventSite:         demoSiteTaipeiHQ,
		StartsAt:          startsAt,
		RegistrationStart: now.Add(-24 * time.Hour),
		RegistrationClose: now.Add(365 * 24 * time.Hour),
		CapacityType:      CapacityTypeLimited,
		Capacity:          30,
		Status:            EventStatusPublished,
		Category:          "demo",
		Tags:              []string{"demo", "check-in"},
		EntryMethod:       "qr",
		Visibility:        "eligible",
		Rule:              RuleInput{Department: "Engineering", Site: demoSiteTaipeiHQ, MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		return "", nil, err
	}
	ticketIDs := make([]string, 0, len(employeeIDs))
	for _, employeeID := range employeeIDs {
		booking, err := s.Book(ctx, Actor{ID: employeeID, Role: RoleEmployee}, event.EventID, BookingRequest{
			EmployeeID:     employeeID,
			IdempotencyKey: fmt.Sprintf("%s-%s", idempotencyPrefix, employeeID),
			FamilyCount:    0,
		})
		if err != nil {
			return "", nil, fmt.Errorf("seed booking %s for event %s: %w", employeeID, event.EventID, err)
		}
		if booking.Ticket == nil {
			return "", nil, conflict(fmt.Sprintf("demo booking %s did not issue a ticket", employeeID))
		}
		ticketIDs = append(ticketIDs, booking.Ticket.TicketID)
	}
	return event.EventID, ticketIDs, nil
}

func demoTodayCheckinStart(now time.Time) time.Time {
	localNow := now.In(checkinBusinessLocation())
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, checkinBusinessLocation()).UTC()
}

func demoNextCheckinStart(todayStarts time.Time) time.Time {
	return todayStarts.In(checkinBusinessLocation()).AddDate(0, 0, 1).UTC()
}
