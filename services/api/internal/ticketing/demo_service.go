package ticketing

import (
	"context"
	"time"
)

const demoSiteTaipeiHQ = "Taipei HQ"

type DemoEventTicketSeed struct {
	TodayEventID   string
	TodayTicketID  string
	FutureEventID  string
	FutureTicketID string
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
		{EmployeeID: "E1003", FullName: "Tainan User", Department: "Engineering", Site: "Tainan HQ", JobGrade: 5, EmploymentStatus: "active"},
		{EmployeeID: "E2001", FullName: "Carla Wu", Department: "Sales", Site: demoSiteTaipeiHQ, JobGrade: 4, EmploymentStatus: "active"},
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
	localNow := s.now().In(checkinBusinessLocation())
	todayStarts := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 12, 0, 0, 0, checkinBusinessLocation()).UTC()
	futureStarts := todayStarts.AddDate(0, 0, 1)

	todayEvent, todayTicket, err := s.seedDemoEventTicket(ctx, "Demo Check-in Today", "demo-today-booking", todayStarts)
	if err != nil {
		return DemoEventTicketSeed{}, err
	}
	futureEvent, futureTicket, err := s.seedDemoEventTicket(ctx, "Demo Future Check-in", "demo-future-booking", futureStarts)
	if err != nil {
		return DemoEventTicketSeed{}, err
	}
	return DemoEventTicketSeed{
		TodayEventID:   todayEvent.EventID,
		TodayTicketID:  todayTicket.TicketID,
		FutureEventID:  futureEvent.EventID,
		FutureTicketID: futureTicket.TicketID,
	}, nil
}

func (s *Service) seedDemoEventTicket(ctx context.Context, title string, idempotencyKey string, startsAt time.Time) (EventSummary, Ticket, error) {
	now := s.now()
	event, err := s.CreateEvent(ctx, Actor{ID: "demo-admin", Role: RoleActivityAdmin}, CreateEventRequest{
		Title:             title,
		Description:       "Demo activity for check-in readiness validation.",
		Location:          demoSiteTaipeiHQ,
		EventCity:         "Taipei",
		EventSite:         demoSiteTaipeiHQ,
		StartsAt:          startsAt,
		RegistrationStart: now.Add(-24 * time.Hour),
		RegistrationClose: now.Add(24 * time.Hour),
		CapacityType:      CapacityTypeLimited,
		Capacity:          20,
		Status:            EventStatusPublished,
		Category:          "demo",
		Tags:              []string{"demo", "check-in"},
		EntryMethod:       "qr",
		Visibility:        "eligible",
		Rule:              RuleInput{Department: "Engineering", Site: demoSiteTaipeiHQ, MinGrade: 5, EmploymentStatus: "active"},
	})
	if err != nil {
		return EventSummary{}, Ticket{}, err
	}
	booking, err := s.Book(ctx, Actor{ID: "E1001", Role: RoleEmployee}, event.EventID, BookingRequest{
		EmployeeID:     "E1001",
		IdempotencyKey: idempotencyKey,
		FamilyCount:    0,
	})
	if err != nil {
		return EventSummary{}, Ticket{}, err
	}
	if booking.Ticket == nil {
		return EventSummary{}, Ticket{}, conflict("demo booking did not issue a ticket")
	}
	return event, *booking.Ticket, nil
}
