package ticketing

import (
	"context"
	"fmt"
	"strings"
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
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			return EventSummary{}, badRequest("title is required")
		}
		event.Title = title
	}
	if req.Description != nil {
		event.Description = *req.Description
	}
	if req.Location != nil {
		event.Location = *req.Location
		if req.EventCity == nil && strings.TrimSpace(storedEventCity) == "" {
			event.EventCity = eventCityOrFallback("", event.Location)
		}
		if req.EventSite == nil && strings.TrimSpace(storedEventSite) == "" {
			event.EventSite = eventSiteOrFallback("", event.Location)
		}
	}
	if req.EventCity != nil {
		event.EventCity = eventCityOrFallback(*req.EventCity, event.Location)
	}
	if req.EventSite != nil {
		event.EventSite = eventSiteOrFallback(*req.EventSite, event.Location)
	}
	if req.StartsAt != nil {
		event.StartsAt = *req.StartsAt
	}
	if req.RegistrationStart != nil {
		event.RegistrationStart = *req.RegistrationStart
	}
	if req.RegistrationClose != nil {
		event.RegistrationClose = *req.RegistrationClose
	}
	if !event.RegistrationStart.Before(event.RegistrationClose) {
		return EventSummary{}, badRequest("registration_start must be before registration_close")
	}
	if req.CapacityType != nil {
		event.CapacityType = strings.TrimSpace(*req.CapacityType)
	}
	if req.capacitySet || req.Capacity != nil {
		event.Capacity = req.Capacity
	}
	if req.AllowsFamily != nil {
		event.AllowsFamily = *req.AllowsFamily
	}
	if event.CapacityType == CapacityTypeUnlimited {
		event.Capacity = nil
		event.AllowsFamily = true
	}
	if err := validateEventCapacity(event); err != nil {
		return EventSummary{}, err
	}
	if event.CapacityType == CapacityTypeLimited {
		confirmed, err := s.confirmedCountTx(ctx, tx, eventID)
		if err != nil {
			return EventSummary{}, err
		}
		if *event.Capacity < confirmed {
			return EventSummary{}, conflict("capacity cannot be lower than confirmed registrations")
		}
	}
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
	event.Version++

	_, err = tx.Exec(ctx, `UPDATE events SET title = $1, description = $2, location = $3, starts_at = $4,
			registration_start = $5, registration_close = $6, capacity_type = $7, capacity = $8, allows_family = $9,
			event_city = $10, event_site = $11, category = $12, tags = $13, entry_method = $14, visibility = $15,
			version = $16, updated_at = now()
		WHERE event_id = $17`,
		event.Title, event.Description, event.Location, event.StartsAt, event.RegistrationStart, event.RegistrationClose, event.CapacityType,
		event.Capacity, event.AllowsFamily, event.EventCity, event.EventSite, event.Category, joinTags(event.Tags), event.EntryMethod, event.Visibility,
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
	if err := insertAudit(ctx, tx, auditID, actor, "event.updated", "event", eventID, map[string]interface{}{"version": event.Version, "capacity_type": event.CapacityType, "capacity": event.Capacity, "allows_family": event.AllowsFamily}); err != nil {
		return EventSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EventSummary{}, err
	}
	return s.GetEventSummary(ctx, actor, eventID, "")
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
	if err := insertAudit(ctx, tx, auditID, actor, "event.state_changed", "event", eventID, map[string]interface{}{"from": previousStatus, "to": next, "reason": req.Reason, "version": event.Version}); err != nil {
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
