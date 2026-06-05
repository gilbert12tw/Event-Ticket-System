package ticketing

// SQL statements for the read-model rebuild admin process (PH2-43).
//
// Rebuild reads OLTP source tables (registrations + employees) as truth and
// writes only reporting projection tables. It never replays outbox history and
// never touches OLTP rows. department is a denormalized TEXT column on
// employees; department_breakdown counts confirmed registrations per label.

// truncateEventSummarySQL clears the projection before repopulating it inside
// the same transaction, so a rollback restores the previous state.
const truncateEventSummarySQL = `TRUNCATE reporting_event_summary`

// aggregateEventSummarySQL aggregates current OLTP truth into one row per
// event. COUNT(*) FILTER is inherently >= 0, satisfying the table CHECK
// constraints. last_event_offset is ” because rebuilt rows are not tied to a
// single outbox event; the worker resumes from the reset projection offset.
const aggregateEventSummarySQL = `
SELECT r.event_id,
       COUNT(*) FILTER (WHERE r.status = 'confirmed')  AS confirmed_count,
       COUNT(*) FILTER (WHERE r.status = 'cancelled')  AS cancelled_count,
       COUNT(*) FILTER (WHERE r.status = 'waitlisted') AS waitlist_count,
       COALESCE(dept.breakdown, '{}'::jsonb)           AS department_breakdown,
       ''                                              AS last_event_offset,
       now()                                           AS updated_at
FROM registrations r
LEFT JOIN (
    SELECT sub.event_id,
           jsonb_object_agg(sub.department, sub.cnt) AS breakdown
    FROM (
        SELECT r2.event_id, emp.department AS department, COUNT(*) AS cnt
        FROM registrations r2
        JOIN employees emp ON emp.employee_id = r2.employee_id
        WHERE r2.status = 'confirmed'
        GROUP BY r2.event_id, emp.department
    ) sub
    GROUP BY sub.event_id
) dept ON dept.event_id = r.event_id
GROUP BY r.event_id, dept.breakdown`

// insertEventSummaryFromOLTPSQL repopulates reporting_event_summary from the
// aggregate query above in a single INSERT ... SELECT.
const insertEventSummaryFromOLTPSQL = `INSERT INTO reporting_event_summary
    (event_id, confirmed_count, cancelled_count, waitlist_count,
     department_breakdown, last_event_offset, updated_at) ` + aggregateEventSummarySQL

// resetProjectionOffsetSQL resets the projection watermark to the current
// outbox head. outbox_id is TEXT, so MAX is lexicographic — consistent with the
// worker's GREATEST(...) advancement. Empty outbox yields ”. RETURNING reports
// the value actually set.
const resetProjectionOffsetSQL = `
INSERT INTO reporting_projection_offsets
    (projection_name, last_processed_outbox_id, updated_at)
VALUES ($1, (SELECT COALESCE(MAX(outbox_id), '') FROM outbox_events), now())
ON CONFLICT (projection_name) DO UPDATE SET
    last_processed_outbox_id = (SELECT COALESCE(MAX(outbox_id), '') FROM outbox_events),
    updated_at = now()
RETURNING last_processed_outbox_id`

// sampleEventIDsSQL selects up to 5 random rebuilt events for spot-check
// validation before commit.
const sampleEventIDsSQL = `SELECT event_id FROM reporting_event_summary ORDER BY random() LIMIT 5`

// summaryConfirmedCountSQL reads the stored confirmed_count for an event.
const summaryConfirmedCountSQL = `SELECT confirmed_count FROM reporting_event_summary WHERE event_id = $1`

// oltpConfirmedCountSQL recounts confirmed registrations from OLTP for an event.
const oltpConfirmedCountSQL = `SELECT COUNT(*) FROM registrations WHERE event_id = $1 AND status = 'confirmed'`
