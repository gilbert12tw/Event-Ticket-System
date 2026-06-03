WITH matching_events AS (
  SELECT event_id, COALESCE(capacity, 0) AS capacity, created_at
  FROM events
  WHERE title = :'event_title'
),
target_event AS (
  SELECT event_id, capacity
  FROM matching_events
  ORDER BY created_at DESC
  LIMIT 1
),
counts AS (
  SELECT
    (SELECT count(*) FROM matching_events) AS matching_events,
    (SELECT COALESCE(max(capacity), 0) FROM target_event) AS capacity,
    (SELECT count(*) FROM registrations r JOIN target_event e ON e.event_id = r.event_id WHERE r.status = 'confirmed') AS confirmed,
    (SELECT count(*) FROM registrations r JOIN target_event e ON e.event_id = r.event_id WHERE r.status = 'waitlisted') AS waitlisted,
    (SELECT count(*) FROM tickets t JOIN target_event e ON e.event_id = t.event_id) AS tickets,
    (SELECT count(*) FROM registrations r JOIN target_event e ON e.event_id = r.event_id LEFT JOIN tickets t ON t.registration_id = r.registration_id WHERE r.status = 'confirmed' AND t.ticket_id IS NULL) AS confirmed_missing_ticket,
    (SELECT count(*) FROM registrations r JOIN target_event e ON e.event_id = r.event_id JOIN tickets t ON t.registration_id = r.registration_id WHERE r.status = 'confirmed' AND t.status <> 'active') AS confirmed_inactive_ticket,
    (SELECT count(*) FROM (
      SELECT r.employee_id
      FROM registrations r JOIN target_event e ON e.event_id = r.event_id
      WHERE r.status IN ('received', 'confirmed', 'waitlisted')
      GROUP BY r.employee_id
      HAVING count(*) > 1
    ) duplicate_active) AS duplicate_active_registrations,
    (SELECT count(*) FROM (
      SELECT cr.ticket_id
      FROM checkin_records cr JOIN tickets t ON t.ticket_id = cr.ticket_id JOIN target_event e ON e.event_id = t.event_id
      GROUP BY cr.ticket_id
      HAVING count(*) > 1
    ) duplicate_checkins) AS duplicate_checkins,
    (SELECT count(*) FROM audit_logs a JOIN registrations r ON r.registration_id = a.entity_id JOIN target_event e ON e.event_id = r.event_id WHERE a.action = 'booking.confirmed') AS booking_confirmed_audits,
    (SELECT count(*) FROM audit_logs a JOIN tickets t ON t.ticket_id = a.entity_id JOIN target_event e ON e.event_id = t.event_id WHERE a.action = 'ticket.issued') AS ticket_issued_audits
)
SELECT label || '|' || value
FROM counts,
LATERAL (VALUES
  ('matching_events', matching_events),
  ('capacity', capacity),
  ('confirmed', confirmed),
  ('waitlisted', waitlisted),
  ('tickets', tickets),
  ('confirmed_missing_ticket', confirmed_missing_ticket),
  ('confirmed_inactive_ticket', confirmed_inactive_ticket),
  ('duplicate_active_registrations', duplicate_active_registrations),
  ('duplicate_checkins', duplicate_checkins),
  ('booking_confirmed_audits', booking_confirmed_audits),
  ('ticket_issued_audits', ticket_issued_audits)
) AS metrics(label, value);
