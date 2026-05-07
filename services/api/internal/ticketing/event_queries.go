package ticketing

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Service) lockEventWithRule(ctx context.Context, tx pgx.Tx, eventID string) (Event, EligibilityRule, error) {
	var event Event
	var tags string
	err := tx.QueryRow(ctx, `SELECT event_id, title, description, location, starts_at, registration_start, registration_close, capacity, status, allocation_mode,
			category, tags, entry_method, visibility, version, COALESCE(archived_at, '0001-01-01 00:00:00+00'::timestamptz), created_by, created_at, updated_at
		FROM events WHERE event_id = $1 FOR UPDATE`, eventID).
		Scan(&event.EventID, &event.Title, &event.Description, &event.Location, &event.StartsAt, &event.RegistrationStart, &event.RegistrationClose, &event.Capacity, &event.Status, &event.AllocationMode,
			&event.Category, &tags, &event.EntryMethod, &event.Visibility, &event.Version, &event.ArchivedAt, &event.CreatedBy, &event.CreatedAt, &event.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, EligibilityRule{}, notFound("event not found")
	}
	if err != nil {
		return Event{}, EligibilityRule{}, err
	}
	event.Tags = splitTags(tags)
	var rule EligibilityRule
	err = tx.QueryRow(ctx, `SELECT rule_id, event_id, department, site, min_grade, employment_status, version FROM eligibility_rules WHERE event_id = $1`, eventID).
		Scan(&rule.RuleID, &rule.EventID, &rule.Department, &rule.Site, &rule.MinGrade, &rule.EmploymentStatus, &rule.Version)
	return event, rule, err
}

func (s *Service) insertEventVersionTx(ctx context.Context, tx pgx.Tx, actor Actor, event Event, reason string) error {
	versionID, err := newID("evv")
	if err != nil {
		return err
	}
	if reason == "" {
		reason = "not specified"
	}
	_, err = tx.Exec(ctx, `INSERT INTO event_versions
		(version_id, event_id, version, title, description, location, starts_at, registration_start, registration_close,
		 capacity, status, allocation_mode, category, tags, entry_method, visibility, changed_by, change_reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		versionID, event.EventID, event.Version, event.Title, event.Description, event.Location, event.StartsAt, event.RegistrationStart, event.RegistrationClose,
		event.Capacity, event.Status, event.AllocationMode, event.Category, joinTags(event.Tags), event.EntryMethod, event.Visibility, actor.ID, reason)
	return err
}

func validEventTransition(from string, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case EventStatusDraft:
		return to == EventStatusPublished || to == EventStatusCancelled || to == EventStatusArchived
	case EventStatusPublished:
		return to == EventStatusClosed || to == EventStatusCancelled || to == EventStatusArchived
	case EventStatusClosed, EventStatusCancelled:
		return to == EventStatusArchived
	default:
		return false
	}
}
