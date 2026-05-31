package ticketing

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type eventRowLockMode string

const (
	eventRowLockShare  eventRowLockMode = " FOR SHARE"
	eventRowLockUpdate eventRowLockMode = " FOR UPDATE"
)

func (s *Service) lockEventWithRule(ctx context.Context, tx pgx.Tx, eventID string) (Event, EligibilityRule, error) {
	return s.readEventWithRuleTx(ctx, tx, eventID, eventRowLockUpdate)
}

// readEventWithRuleShareLockTx is used by the PH2-22 exhausted fast path.
// Waitlist inserts have no capacity constraint, so they do not need the
// exclusive `FOR UPDATE` lock, but they still take a shared lock to serialize
// with event state changes such as close, cancel, or archive.
func (s *Service) readEventWithRuleShareLockTx(ctx context.Context, tx pgx.Tx, eventID string) (Event, EligibilityRule, error) {
	return s.readEventWithRuleTx(ctx, tx, eventID, eventRowLockShare)
}

func (s *Service) readEventWithRuleTx(ctx context.Context, tx pgx.Tx, eventID string, lockMode eventRowLockMode) (Event, EligibilityRule, error) {
	var event Event
	var tags string
	var capacity pgtype.Int4
	query := `SELECT event_id, title, description, location, event_city, event_site, starts_at, registration_start, registration_close,
			capacity_type, capacity, allows_family, status, allocation_mode,
			category, tags, entry_method, visibility, version, COALESCE(archived_at, '0001-01-01 00:00:00+00'::timestamptz), created_by, created_at, updated_at
		FROM events WHERE event_id = $1`
	query += string(lockMode)
	err := tx.QueryRow(ctx, query, eventID).
		Scan(&event.EventID, &event.Title, &event.Description, &event.Location, &event.EventCity, &event.EventSite, &event.StartsAt, &event.RegistrationStart, &event.RegistrationClose,
			&event.CapacityType, &capacity, &event.AllowsFamily, &event.Status, &event.AllocationMode,
			&event.Category, &tags, &event.EntryMethod, &event.Visibility, &event.Version, &event.ArchivedAt, &event.CreatedBy, &event.CreatedAt, &event.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, EligibilityRule{}, notFound(errEventNotFoundMessage)
	}
	if err != nil {
		return Event{}, EligibilityRule{}, err
	}
	if capacity.Valid {
		value := int(capacity.Int32)
		event.Capacity = &value
	}
	event.EventCity = eventCityOrFallback(event.EventCity, event.Location)
	event.EventSite = eventSiteOrFallback(event.EventSite, event.Location)
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
		(version_id, event_id, version, title, description, location, event_city, event_site, starts_at, registration_start, registration_close,
		 capacity_type, capacity, allows_family, status, allocation_mode, category, tags, entry_method, visibility, changed_by, change_reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)`,
		versionID, event.EventID, event.Version, event.Title, event.Description, event.Location, event.EventCity, event.EventSite, event.StartsAt, event.RegistrationStart, event.RegistrationClose,
		event.CapacityType, event.Capacity, event.AllowsFamily, event.Status, event.AllocationMode, event.Category, joinTags(event.Tags), event.EntryMethod, event.Visibility, actor.ID, reason)
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
