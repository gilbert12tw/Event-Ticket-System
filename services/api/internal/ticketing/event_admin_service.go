package ticketing

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Service) ListAdminEvents(ctx context.Context, actor Actor) ([]EventSummary, error) {
	if err := requireAnyRole(actor, RoleActivityAdmin, RoleHRAdmin, RoleCheckinStaff); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT event_id FROM events WHERE archived_at IS NULL ORDER BY starts_at DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []EventSummary
	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			return nil, err
		}
		summary, err := s.GetEventSummary(ctx, actor, eventID, "")
		if err != nil {
			return nil, err
		}
		events = append(events, summary)
	}
	return events, rows.Err()
}

func (s *Service) GetEvent(ctx context.Context, actor Actor, eventID string, employeeID string) (EventSummary, error) {
	if actor.Role == RoleEmployee {
		employeeID = actor.ID
	} else if err := requireAnyRole(actor, RoleActivityAdmin, RoleHRAdmin, RoleCheckinStaff); err != nil {
		return EventSummary{}, err
	}
	return s.GetEventSummary(ctx, actor, eventID, employeeID)
}

func (s *Service) UpdateEvent(ctx context.Context, actor Actor, eventID string, req UpdateEventRequest) (EventSummary, error) {
	if err := requireRole(actor, RoleActivityAdmin); err != nil {
		return EventSummary{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EventSummary{}, err
	}
	defer rollback(ctx, tx)

	event, _, err := s.lockEventWithRule(ctx, tx, eventID)
	if err != nil {
		return EventSummary{}, err
	}
	var storedEventCity string
	var storedEventSite string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(event_city, ''), COALESCE(event_site, '') FROM events WHERE event_id = $1`, eventID).Scan(&storedEventCity, &storedEventSite); err != nil {
		return EventSummary{}, err
	}
	if event.Status == EventStatusArchived || !event.ArchivedAt.IsZero() {
		return EventSummary{}, conflict("archived events cannot be edited")
	}
	if err := s.applyEventUpdateRequest(ctx, tx, &event, req, storedEventCity, storedEventSite); err != nil {
		return EventSummary{}, err
	}
	event.Version++

	_, err = tx.Exec(ctx, `UPDATE events SET title = $1, description = $2, location = $3, starts_at = $4,
			registration_start = $5, registration_close = $6, capacity_type = $7, capacity = $8, allows_family = $9,
			allocation_mode = $10, event_city = $11, event_site = $12, category = $13, tags = $14, entry_method = $15, visibility = $16,
			version = $17, updated_at = now()
		WHERE event_id = $18`,
		event.Title, event.Description, event.Location, event.StartsAt, event.RegistrationStart, event.RegistrationClose, event.CapacityType,
		event.Capacity, event.AllowsFamily, event.AllocationMode, event.EventCity, event.EventSite, event.Category, joinTags(event.Tags), event.EntryMethod, event.Visibility,
		event.Version, eventID)
	if err != nil {
		return EventSummary{}, err
	}
	if err := s.insertEventVersionTx(ctx, tx, actor, event, "event updated"); err != nil {
		return EventSummary{}, err
	}
	auditID, err := newID("aud")
	if err != nil {
		return EventSummary{}, err
	}
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor, "event.updated", "event", eventID, map[string]interface{}{"version": event.Version, "capacity_type": event.CapacityType, "capacity": event.Capacity, "allows_family": event.AllowsFamily, "allocation_mode": event.AllocationMode})); err != nil {
		return EventSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EventSummary{}, err
	}
	return s.GetEventSummary(ctx, actor, eventID, "")
}

func (s *Service) applyEventUpdateRequest(ctx context.Context, tx pgx.Tx, event *Event, req UpdateEventRequest, storedEventCity string, storedEventSite string) error {
	if err := applyEventContentUpdate(event, req, storedEventCity, storedEventSite); err != nil {
		return err
	}
	applyEventScheduleUpdate(event, req)
	if !event.RegistrationStart.Before(event.RegistrationClose) {
		return badRequest("registration_start must be before registration_close")
	}
	if err := s.applyEventCapacityUpdate(ctx, tx, event, req); err != nil {
		return err
	}
	applyEventMetadataUpdate(event, req)
	return nil
}

func applyEventContentUpdate(event *Event, req UpdateEventRequest, storedEventCity string, storedEventSite string) error {
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			return badRequest("title is required")
		}
		event.Title = title
	}
	if req.Description != nil {
		event.Description = *req.Description
	}
	if req.Location != nil {
		event.Location = *req.Location
		applyLocationDerivedFields(event, req, storedEventCity, storedEventSite)
	}
	if req.EventCity != nil {
		event.EventCity = eventCityOrFallback(*req.EventCity, event.Location)
	}
	if req.EventSite != nil {
		event.EventSite = eventSiteOrFallback(*req.EventSite, event.Location)
	}
	return nil
}

func applyLocationDerivedFields(event *Event, req UpdateEventRequest, storedEventCity string, storedEventSite string) {
	if req.EventCity == nil && strings.TrimSpace(storedEventCity) == "" {
		event.EventCity = eventCityOrFallback("", event.Location)
	}
	if req.EventSite == nil && strings.TrimSpace(storedEventSite) == "" {
		event.EventSite = eventSiteOrFallback("", event.Location)
	}
}

func applyEventScheduleUpdate(event *Event, req UpdateEventRequest) {
	if req.StartsAt != nil {
		event.StartsAt = *req.StartsAt
	}
	if req.RegistrationStart != nil {
		event.RegistrationStart = *req.RegistrationStart
	}
	if req.RegistrationClose != nil {
		event.RegistrationClose = *req.RegistrationClose
	}
}

func (s *Service) applyEventCapacityUpdate(ctx context.Context, tx pgx.Tx, event *Event, req UpdateEventRequest) error {
	if req.CapacityType != nil {
		event.CapacityType = strings.TrimSpace(*req.CapacityType)
	}
	if req.capacitySet || req.Capacity != nil {
		event.Capacity = req.Capacity
	}
	if req.AllowsFamily != nil {
		event.AllowsFamily = *req.AllowsFamily
	}
	if err := s.applyEventAllocationModeUpdate(ctx, tx, event, req.AllocationMode); err != nil {
		return err
	}
	if event.CapacityType == CapacityTypeUnlimited {
		event.Capacity = nil
		event.AllowsFamily = true
	}
	if err := validateEventCapacity(*event); err != nil {
		return err
	}
	if err := validateAllocationModeForCapacity(event.AllocationMode, event.CapacityType); err != nil {
		return err
	}
	return s.validateLimitedEventUpdateTx(ctx, tx, *event)
}

func (s *Service) applyEventAllocationModeUpdate(ctx context.Context, tx pgx.Tx, event *Event, mode *string) error {
	if mode == nil {
		return nil
	}
	allocationMode, err := normalizeAllocationMode(*mode)
	if err != nil {
		return err
	}
	if allocationMode == event.AllocationMode {
		return nil
	}
	registrations, err := s.registrationCountTx(ctx, tx, event.EventID)
	if err != nil {
		return err
	}
	if registrations > 0 {
		return conflict("allocation_mode cannot change after registrations exist")
	}
	event.AllocationMode = allocationMode
	return nil
}

func (s *Service) validateLimitedEventUpdateTx(ctx context.Context, tx pgx.Tx, event Event) error {
	if event.CapacityType != CapacityTypeLimited {
		return nil
	}
	confirmed, err := s.confirmedCountTx(ctx, tx, event.EventID)
	if err != nil {
		return err
	}
	if *event.Capacity < confirmed {
		return conflict("capacity cannot be lower than confirmed registrations")
	}
	familyRegistrations, err := s.activeFamilyRegistrationCountTx(ctx, tx, event.EventID)
	if err != nil {
		return err
	}
	if familyRegistrations > 0 {
		return conflict("limited events cannot contain family registrations")
	}
	return nil
}

func applyEventMetadataUpdate(event *Event, req UpdateEventRequest) {
	if req.Category != nil {
		event.Category = strings.TrimSpace(*req.Category)
	}
	if req.Tags != nil {
		event.Tags = normalizeTags(req.Tags)
	}
	if req.EntryMethod != nil {
		event.EntryMethod = strings.TrimSpace(*req.EntryMethod)
	}
	if event.EntryMethod == "" {
		event.EntryMethod = "qr"
	}
	if req.Visibility != nil {
		event.Visibility = strings.TrimSpace(*req.Visibility)
	}
	if event.Visibility == "" {
		event.Visibility = "eligible"
	}
}

func (s *Service) ChangeEventState(ctx context.Context, actor Actor, eventID string, req ChangeEventStateRequest) (EventSummary, error) {
	if err := requireRole(actor, RoleActivityAdmin); err != nil {
		return EventSummary{}, err
	}
	next := strings.TrimSpace(req.Status)
	if next == "" {
		return EventSummary{}, badRequest("status is required")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EventSummary{}, err
	}
	defer rollback(ctx, tx)

	event, _, err := s.lockEventWithRule(ctx, tx, eventID)
	if err != nil {
		return EventSummary{}, err
	}
	previousStatus := event.Status
	if !validEventTransition(event.Status, next) {
		return EventSummary{}, conflict(fmt.Sprintf("cannot transition event from %s to %s", event.Status, next))
	}
	event.Status = next
	event.Version++
	if next == EventStatusArchived {
		_, err = tx.Exec(ctx, `UPDATE events SET status = $1, version = $2, archived_at = now(), updated_at = now() WHERE event_id = $3`, next, event.Version, eventID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE events SET status = $1, version = $2, updated_at = now() WHERE event_id = $3`, next, event.Version, eventID)
	}
	if err != nil {
		return EventSummary{}, err
	}
	if err := s.insertEventVersionTx(ctx, tx, actor, event, req.Reason); err != nil {
		return EventSummary{}, err
	}
	auditID, err := newID("aud")
	if err != nil {
		return EventSummary{}, err
	}
	if err := insertAudit(ctx, tx, newAuditRecord(auditID, actor, "event.state_changed", "event", eventID, map[string]interface{}{"from": previousStatus, "to": next, "reason": req.Reason, "version": event.Version})); err != nil {
		return EventSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EventSummary{}, err
	}
	return s.GetEventSummary(ctx, actor, eventID, "")
}

func (s *Service) DuplicateEvent(ctx context.Context, actor Actor, eventID string) (EventSummary, error) {
	if err := requireRole(actor, RoleActivityAdmin); err != nil {
		return EventSummary{}, err
	}
	source, err := s.GetEventSummary(ctx, actor, eventID, "")
	if err != nil {
		return EventSummary{}, err
	}
	return s.CreateEvent(ctx, actor, CreateEventRequest{
		Title:             "Copy of " + source.Title,
		Description:       source.Description,
		Location:          source.Location,
		EventCity:         source.EventCity,
		EventSite:         source.EventSite,
		StartsAt:          source.StartsAt,
		RegistrationStart: source.RegistrationStart,
		RegistrationClose: source.RegistrationClose,
		CapacityType:      source.CapacityType,
		Capacity:          capacityValue(source.Capacity),
		AllowsFamily:      source.AllowsFamily,
		AllocationMode:    source.AllocationMode,
		Status:            EventStatusDraft,
		Category:          source.Category,
		Tags:              source.Tags,
		EntryMethod:       source.EntryMethod,
		Visibility:        source.Visibility,
		Rule: RuleInput{
			Department:       source.Rule.Department,
			Site:             source.Rule.Site,
			MinGrade:         source.Rule.MinGrade,
			EmploymentStatus: source.Rule.EmploymentStatus,
		},
	})
}

func (s *Service) ArchiveEvent(ctx context.Context, actor Actor, eventID string) (EventSummary, error) {
	return s.ChangeEventState(ctx, actor, eventID, ChangeEventStateRequest{Status: EventStatusArchived, Reason: "archive requested"})
}
