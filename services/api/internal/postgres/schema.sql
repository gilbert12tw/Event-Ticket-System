CREATE TABLE IF NOT EXISTS employees (
		employee_id TEXT PRIMARY KEY,
		full_name TEXT NOT NULL,
		department TEXT NOT NULL,
		site TEXT NOT NULL,
		job_grade INTEGER NOT NULL,
		employment_status TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

CREATE TABLE IF NOT EXISTS events (
		event_id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		location TEXT NOT NULL DEFAULT '',
		event_city TEXT NOT NULL DEFAULT '',
		event_site TEXT NOT NULL DEFAULT '',
		starts_at TIMESTAMPTZ NOT NULL,
		ends_at TIMESTAMPTZ NOT NULL,
		registration_start TIMESTAMPTZ NOT NULL,
		registration_close TIMESTAMPTZ NOT NULL,
		capacity_type TEXT NOT NULL DEFAULT 'limited' CHECK (capacity_type IN ('limited', 'unlimited')),
		capacity INTEGER,
		allows_family BOOLEAN NOT NULL DEFAULT false,
		status TEXT NOT NULL CHECK (status IN ('draft', 'published', 'closed', 'cancelled', 'archived')),
		allocation_mode TEXT NOT NULL DEFAULT 'fcfs',
		category TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '',
		entry_method TEXT NOT NULL DEFAULT 'qr',
		visibility TEXT NOT NULL DEFAULT 'eligible',
		version INTEGER NOT NULL DEFAULT 1,
		archived_at TIMESTAMPTZ,
		created_by TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		CONSTRAINT events_capacity_rules_check CHECK (
			(capacity_type = 'limited' AND capacity IS NOT NULL AND capacity > 0 AND allows_family = false)
			OR (capacity_type = 'unlimited' AND capacity IS NULL)
		),
		CONSTRAINT events_time_window_check CHECK (
			ends_at > starts_at
		)
	);

CREATE TABLE IF NOT EXISTS event_versions (
		version_id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		version INTEGER NOT NULL,
		title TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		location TEXT NOT NULL DEFAULT '',
		event_city TEXT NOT NULL DEFAULT '',
		event_site TEXT NOT NULL DEFAULT '',
		starts_at TIMESTAMPTZ NOT NULL,
		ends_at TIMESTAMPTZ NOT NULL,
		registration_start TIMESTAMPTZ NOT NULL,
		registration_close TIMESTAMPTZ NOT NULL,
		capacity_type TEXT NOT NULL DEFAULT 'limited' CHECK (capacity_type IN ('limited', 'unlimited')),
		capacity INTEGER,
		allows_family BOOLEAN NOT NULL DEFAULT false,
		status TEXT NOT NULL,
		allocation_mode TEXT NOT NULL DEFAULT 'fcfs',
		category TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '',
		entry_method TEXT NOT NULL DEFAULT 'qr',
		visibility TEXT NOT NULL DEFAULT 'eligible',
		changed_by TEXT NOT NULL,
		change_reason TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		CONSTRAINT event_versions_capacity_rules_check CHECK (
			(capacity_type = 'limited' AND capacity IS NOT NULL AND capacity > 0 AND allows_family = false)
			OR (capacity_type = 'unlimited' AND capacity IS NULL)
		),
		CONSTRAINT event_versions_time_window_check CHECK (
			ends_at > starts_at
		),
		UNIQUE (event_id, version)
	);

CREATE TABLE IF NOT EXISTS event_assets (
		asset_id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		object_key TEXT NOT NULL,
		file_name TEXT NOT NULL,
		content_type TEXT NOT NULL,
		size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
		created_by TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

CREATE TABLE IF NOT EXISTS eligibility_rules (
		rule_id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		department TEXT NOT NULL DEFAULT '*',
		site TEXT NOT NULL DEFAULT '*',
		min_grade INTEGER NOT NULL DEFAULT 0,
		employment_status TEXT NOT NULL DEFAULT 'active',
		version INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

CREATE TABLE IF NOT EXISTS eligibility_rule_versions (
		version_id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		version INTEGER NOT NULL,
		expression_json JSONB NOT NULL,
		match_count INTEGER NOT NULL DEFAULT 0,
		created_by TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		UNIQUE (event_id, version)
	);

CREATE TABLE IF NOT EXISTS hr_sync_batches (
		batch_id TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('pending', 'applied', 'failed')),
		employee_count INTEGER NOT NULL DEFAULT 0,
		started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		completed_at TIMESTAMPTZ
	);

CREATE TABLE IF NOT EXISTS registrations (
		registration_id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		employee_id TEXT NOT NULL REFERENCES employees(employee_id),
		status TEXT NOT NULL CHECK (status IN ('received', 'confirmed', 'waitlisted', 'cancelled')),
		idempotency_key TEXT NOT NULL UNIQUE,
		rejection_reason TEXT NOT NULL DEFAULT '',
		cancel_idempotency_key TEXT,
		cancel_reason TEXT NOT NULL DEFAULT '',
		cancelled_at TIMESTAMPTZ,
		family_count INTEGER NOT NULL DEFAULT 0 CHECK (family_count BETWEEN 0 AND 10),
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

CREATE TABLE IF NOT EXISTS tickets (
		ticket_id TEXT PRIMARY KEY,
		registration_id TEXT NOT NULL UNIQUE REFERENCES registrations(registration_id) ON DELETE CASCADE,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		employee_id TEXT NOT NULL REFERENCES employees(employee_id),
		status TEXT NOT NULL CHECK (status IN ('active', 'redeemed', 'revoked', 'expired')),
		sequence_number INTEGER NOT NULL DEFAULT 1,
		signed_token_hash TEXT NOT NULL UNIQUE,
		signed_token TEXT NOT NULL DEFAULT '',
		qr_payload TEXT NOT NULL DEFAULT '',
		expires_at TIMESTAMPTZ,
		revoked_reason TEXT NOT NULL DEFAULT '',
		issued_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

CREATE TABLE IF NOT EXISTS booking_idempotency_results (
		idempotency_key TEXT PRIMARY KEY,
		event_id TEXT NOT NULL,
		employee_id TEXT NOT NULL,
		family_count INTEGER NOT NULL DEFAULT 0 CHECK (family_count BETWEEN 0 AND 10),
		registration_id TEXT REFERENCES registrations(registration_id) ON DELETE CASCADE,
		registration_status TEXT NOT NULL DEFAULT '' CHECK (registration_status IN ('', 'received', 'confirmed', 'waitlisted', 'cancelled')),
		ticket_id TEXT REFERENCES tickets(ticket_id) ON DELETE SET NULL,
		remaining_capacity INTEGER NOT NULL DEFAULT 0 CHECK (remaining_capacity >= 0),
		message TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		completed_at TIMESTAMPTZ
	);

CREATE TABLE IF NOT EXISTS eligibility_impact_reviews (
		review_id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		employee_id TEXT NOT NULL REFERENCES employees(employee_id),
		ticket_id TEXT REFERENCES tickets(ticket_id) ON DELETE SET NULL,
		status TEXT NOT NULL CHECK (status IN ('pending', 'resolved')),
		reason TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		resolved_at TIMESTAMPTZ
	);

CREATE TABLE IF NOT EXISTS checkin_records (
		checkin_id TEXT PRIMARY KEY,
		ticket_id TEXT NOT NULL UNIQUE REFERENCES tickets(ticket_id) ON DELETE CASCADE,
		staff_id TEXT NOT NULL,
		device_id TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('accepted', 'conflict')),
		scanned_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

CREATE TABLE IF NOT EXISTS checkin_rejections (
		rejection_id TEXT PRIMARY KEY,
		ticket_id TEXT REFERENCES tickets(ticket_id) ON DELETE SET NULL,
		event_id TEXT NOT NULL DEFAULT '',
		staff_id TEXT NOT NULL,
		device_id TEXT NOT NULL,
		reason TEXT NOT NULL,
		detail TEXT NOT NULL DEFAULT '',
		rejected_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

CREATE TABLE IF NOT EXISTS no_show_records (
		no_show_id TEXT PRIMARY KEY,
		registration_id TEXT NOT NULL UNIQUE REFERENCES registrations(registration_id) ON DELETE CASCADE,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		employee_id TEXT NOT NULL REFERENCES employees(employee_id),
		status TEXT NOT NULL CHECK (status IN ('recorded', 'cooldown_active', 'cooldown_expired')),
		recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		cooldown_until TIMESTAMPTZ
	);

CREATE TABLE IF NOT EXISTS audit_logs (
		audit_id TEXT PRIMARY KEY,
		actor_id TEXT NOT NULL,
		role TEXT NOT NULL,
		action TEXT NOT NULL,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

CREATE TABLE IF NOT EXISTS outbox_events (
		outbox_id TEXT PRIMARY KEY,
		aggregate_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		payload JSONB NOT NULL,
		publish_status TEXT NOT NULL DEFAULT 'pending',
		schema_version INTEGER NOT NULL DEFAULT 1,
		idempotency_key TEXT,
		partition_key TEXT,
		attempts INTEGER NOT NULL DEFAULT 0,
		retry_count INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		dead_letter_at TIMESTAMPTZ,
		available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		lease_started_at TIMESTAMPTZ,
		published_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

CREATE TABLE IF NOT EXISTS notification_preferences (
		employee_id TEXT PRIMARY KEY REFERENCES employees(employee_id) ON DELETE CASCADE,
		email_enabled BOOLEAN NOT NULL DEFAULT true,
		in_app_enabled BOOLEAN NOT NULL DEFAULT true,
		opted_out_categories TEXT NOT NULL DEFAULT '',
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

CREATE TABLE IF NOT EXISTS notification_deliveries (
		delivery_id TEXT PRIMARY KEY,
		outbox_id TEXT REFERENCES outbox_events(outbox_id) ON DELETE SET NULL,
		employee_id TEXT REFERENCES employees(employee_id) ON DELETE SET NULL,
		channel TEXT NOT NULL CHECK (channel IN ('email', 'in_app')),
		status TEXT NOT NULL CHECK (status IN ('pending', 'sending', 'sent', 'failed', 'suppressed', 'dead_letter')),
		attempts INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

CREATE TABLE IF NOT EXISTS lottery_runs (
		run_id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		seed TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('completed', 'superseded')),
		input_snapshot_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		algorithm_version TEXT NOT NULL DEFAULT 'deterministic-sha256-v1',
		candidate_count INTEGER NOT NULL DEFAULT 0 CHECK (candidate_count >= 0),
		eligibility_rule_id TEXT NOT NULL DEFAULT '',
		eligibility_rule_version INTEGER NOT NULL DEFAULT 0,
		eligibility_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
		winner_count INTEGER NOT NULL DEFAULT 0,
		created_by TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		UNIQUE (event_id, seed)
	);

CREATE TABLE IF NOT EXISTS lottery_results (
		result_id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL REFERENCES lottery_runs(run_id) ON DELETE CASCADE,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		registration_id TEXT NOT NULL REFERENCES registrations(registration_id) ON DELETE CASCADE,
		employee_id TEXT NOT NULL REFERENCES employees(employee_id),
		result TEXT NOT NULL CHECK (result IN ('winner', 'waitlisted')),
		ticket_id TEXT REFERENCES tickets(ticket_id) ON DELETE SET NULL,
		draw_order INTEGER NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		UNIQUE (run_id, registration_id)
	);

CREATE TABLE IF NOT EXISTS offline_checkin_batches (
		batch_id TEXT PRIMARY KEY,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		device_id TEXT NOT NULL,
		staff_id TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('open', 'synced', 'conflict')),
		valid_until TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '4 hours'),
		package_signature TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		synced_at TIMESTAMPTZ
	);

CREATE TABLE IF NOT EXISTS offline_checkin_scans (
		scan_id TEXT PRIMARY KEY,
		batch_id TEXT NOT NULL REFERENCES offline_checkin_batches(batch_id) ON DELETE CASCADE,
		ticket_id TEXT REFERENCES tickets(ticket_id) ON DELETE CASCADE,
		device_id TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('accepted', 'duplicate', 'conflict')),
		token_hash TEXT NOT NULL DEFAULT '',
		conflict_reason TEXT NOT NULL DEFAULT '',
		scanned_at TIMESTAMPTZ NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

CREATE TABLE IF NOT EXISTS report_exports (
		export_id TEXT PRIMARY KEY,
		requested_by TEXT NOT NULL,
		report_type TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('pending', 'ready', 'failed')),
		object_key TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		completed_at TIMESTAMPTZ
	);

CREATE INDEX IF NOT EXISTS idx_events_status ON events(status);

CREATE INDEX IF NOT EXISTS idx_event_versions_event ON event_versions(event_id, version DESC);

CREATE INDEX IF NOT EXISTS idx_eligibility_rule_versions_event ON eligibility_rule_versions(event_id, version DESC);

CREATE INDEX IF NOT EXISTS idx_eligibility_impact_reviews_status ON eligibility_impact_reviews(status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_registrations_event_status ON registrations(event_id, status);

CREATE INDEX IF NOT EXISTS idx_registrations_employee_status ON registrations(employee_id, status);

CREATE INDEX IF NOT EXISTS idx_no_show_records_employee ON no_show_records(employee_id, cooldown_until DESC);

CREATE INDEX IF NOT EXISTS idx_tickets_employee ON tickets(employee_id);

CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_filter ON audit_logs(action, entity_type, entity_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_rate_limited_event_recent
	ON audit_logs(created_at DESC, (metadata->>'event_id'))
	WHERE action = 'booking.rate_limited' AND metadata ? 'event_id';

CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox_events(publish_status, available_at);

CREATE INDEX IF NOT EXISTS idx_outbox_published_lag ON outbox_events(event_type, published_at, created_at)
		WHERE publish_status = 'published' AND published_at IS NOT NULL;

-- Supports the PH2-43 rebuild watermark lookup (newest event by created_at,
-- outbox_id) so it is an index scan + LIMIT 1 rather than a full sort.
CREATE INDEX IF NOT EXISTS idx_outbox_created_at ON outbox_events(created_at DESC, outbox_id DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_deliveries_outbox_channel ON notification_deliveries(outbox_id, channel) WHERE outbox_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_status ON notification_deliveries(status, updated_at DESC);

ALTER TABLE notification_deliveries DROP CONSTRAINT IF EXISTS notification_deliveries_status_check;

ALTER TABLE notification_deliveries ADD CONSTRAINT notification_deliveries_status_check CHECK (status IN ('pending', 'sending', 'sent', 'failed', 'suppressed', 'dead_letter'));

CREATE INDEX IF NOT EXISTS idx_lottery_runs_event ON lottery_runs(event_id, created_at DESC);

ALTER TABLE lottery_runs DROP CONSTRAINT IF EXISTS lottery_runs_status_check;

ALTER TABLE lottery_runs ADD CONSTRAINT lottery_runs_status_check CHECK (status IN ('completed', 'superseded'));

WITH ranked_completed_lottery_runs AS (
		SELECT run_id,
			row_number() OVER (PARTITION BY event_id ORDER BY created_at DESC, run_id DESC) AS rank
		FROM lottery_runs
		WHERE status = 'completed'
	)
	UPDATE lottery_runs lr
	SET status = 'superseded'
	FROM ranked_completed_lottery_runs ranked
	WHERE lr.run_id = ranked.run_id
		AND ranked.rank > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_runs_one_completed_event
		ON lottery_runs (event_id)
		WHERE status = 'completed';

CREATE INDEX IF NOT EXISTS idx_lottery_results_run ON lottery_results(run_id, draw_order);

ALTER TABLE lottery_runs ADD COLUMN IF NOT EXISTS input_snapshot_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE lottery_runs ADD COLUMN IF NOT EXISTS algorithm_version TEXT NOT NULL DEFAULT 'deterministic-sha256-v1';

ALTER TABLE lottery_runs ADD COLUMN IF NOT EXISTS candidate_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE lottery_runs ADD COLUMN IF NOT EXISTS eligibility_rule_id TEXT NOT NULL DEFAULT '';

ALTER TABLE lottery_runs ADD COLUMN IF NOT EXISTS eligibility_rule_version INTEGER NOT NULL DEFAULT 0;

ALTER TABLE lottery_runs ADD COLUMN IF NOT EXISTS eligibility_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE lottery_runs DROP CONSTRAINT IF EXISTS lottery_runs_candidate_count_check;

ALTER TABLE lottery_runs ADD CONSTRAINT lottery_runs_candidate_count_check CHECK (candidate_count >= 0);

ALTER TABLE events DROP CONSTRAINT IF EXISTS events_status_check;

ALTER TABLE events ADD CONSTRAINT events_status_check CHECK (status IN ('draft', 'published', 'closed', 'cancelled', 'archived'));

ALTER TABLE events ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT '';

ALTER TABLE events ADD COLUMN IF NOT EXISTS event_city TEXT NOT NULL DEFAULT '';

ALTER TABLE events ADD COLUMN IF NOT EXISTS event_site TEXT NOT NULL DEFAULT '';

ALTER TABLE events ADD COLUMN IF NOT EXISTS ends_at TIMESTAMPTZ;

UPDATE events SET ends_at = starts_at + interval '24 hours' WHERE ends_at IS NULL;

ALTER TABLE events ALTER COLUMN ends_at SET NOT NULL;

ALTER TABLE events DROP CONSTRAINT IF EXISTS events_time_window_check;

ALTER TABLE events ADD CONSTRAINT events_time_window_check CHECK (ends_at > starts_at);

ALTER TABLE events ADD COLUMN IF NOT EXISTS capacity_type TEXT NOT NULL DEFAULT 'limited';

ALTER TABLE events ADD COLUMN IF NOT EXISTS allows_family BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE events DROP CONSTRAINT IF EXISTS events_capacity_check;

ALTER TABLE events DROP CONSTRAINT IF EXISTS events_capacity_type_check;

ALTER TABLE events DROP CONSTRAINT IF EXISTS events_capacity_rules_check;

ALTER TABLE events ALTER COLUMN capacity DROP NOT NULL;

UPDATE events SET capacity_type = 'limited' WHERE capacity_type = '';

UPDATE events SET allows_family = false WHERE capacity_type = 'limited';

ALTER TABLE events ADD CONSTRAINT events_capacity_type_check CHECK (capacity_type IN ('limited', 'unlimited'));

ALTER TABLE events ADD CONSTRAINT events_capacity_rules_check CHECK (
		(capacity_type = 'limited' AND capacity IS NOT NULL AND capacity > 0 AND allows_family = false)
		OR (capacity_type = 'unlimited' AND capacity IS NULL)
	);

ALTER TABLE events ADD COLUMN IF NOT EXISTS tags TEXT NOT NULL DEFAULT '';

ALTER TABLE events ADD COLUMN IF NOT EXISTS entry_method TEXT NOT NULL DEFAULT 'qr';

ALTER TABLE events ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'eligible';

ALTER TABLE events ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1;

ALTER TABLE events ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

ALTER TABLE event_versions ADD COLUMN IF NOT EXISTS event_city TEXT NOT NULL DEFAULT '';

ALTER TABLE event_versions ADD COLUMN IF NOT EXISTS event_site TEXT NOT NULL DEFAULT '';

ALTER TABLE event_versions ADD COLUMN IF NOT EXISTS ends_at TIMESTAMPTZ;

UPDATE event_versions SET ends_at = starts_at + interval '24 hours' WHERE ends_at IS NULL;

ALTER TABLE event_versions ALTER COLUMN ends_at SET NOT NULL;

ALTER TABLE event_versions DROP CONSTRAINT IF EXISTS event_versions_time_window_check;

ALTER TABLE event_versions ADD CONSTRAINT event_versions_time_window_check CHECK (ends_at > starts_at);

ALTER TABLE event_versions ADD COLUMN IF NOT EXISTS capacity_type TEXT NOT NULL DEFAULT 'limited';

ALTER TABLE event_versions ADD COLUMN IF NOT EXISTS allows_family BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE event_versions DROP CONSTRAINT IF EXISTS event_versions_capacity_check;

ALTER TABLE event_versions DROP CONSTRAINT IF EXISTS event_versions_capacity_type_check;

ALTER TABLE event_versions DROP CONSTRAINT IF EXISTS event_versions_capacity_rules_check;

ALTER TABLE event_versions ALTER COLUMN capacity DROP NOT NULL;

UPDATE event_versions SET capacity_type = 'limited' WHERE capacity_type = '';

UPDATE event_versions SET allows_family = false WHERE capacity_type = 'limited';

ALTER TABLE event_versions ADD CONSTRAINT event_versions_capacity_type_check CHECK (capacity_type IN ('limited', 'unlimited'));

ALTER TABLE event_versions ADD CONSTRAINT event_versions_capacity_rules_check CHECK (
		(capacity_type = 'limited' AND capacity IS NOT NULL AND capacity > 0 AND allows_family = false)
		OR (capacity_type = 'unlimited' AND capacity IS NULL)
	);

ALTER TABLE registrations ADD COLUMN IF NOT EXISTS cancel_idempotency_key TEXT;

ALTER TABLE registrations ADD COLUMN IF NOT EXISTS cancel_reason TEXT NOT NULL DEFAULT '';

ALTER TABLE registrations ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;

ALTER TABLE registrations ADD COLUMN IF NOT EXISTS family_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE registrations DROP CONSTRAINT IF EXISTS registrations_family_count_check;

ALTER TABLE registrations ADD CONSTRAINT registrations_family_count_check CHECK (family_count BETWEEN 0 AND 10);

ALTER TABLE registrations DROP CONSTRAINT IF EXISTS registrations_status_check;

ALTER TABLE registrations ADD CONSTRAINT registrations_status_check CHECK (status IN ('received', 'confirmed', 'waitlisted', 'cancelled'));

ALTER TABLE registrations DROP CONSTRAINT IF EXISTS registrations_event_id_employee_id_key;

CREATE UNIQUE INDEX IF NOT EXISTS registrations_unique_active_employee
		ON registrations (event_id, employee_id)
		WHERE status <> 'cancelled';

CREATE TABLE IF NOT EXISTS booking_idempotency_results (
		idempotency_key TEXT PRIMARY KEY,
		event_id TEXT NOT NULL,
		employee_id TEXT NOT NULL,
		family_count INTEGER NOT NULL DEFAULT 0 CHECK (family_count BETWEEN 0 AND 10),
		registration_id TEXT REFERENCES registrations(registration_id) ON DELETE CASCADE,
		registration_status TEXT NOT NULL DEFAULT '' CHECK (registration_status IN ('', 'received', 'confirmed', 'waitlisted', 'cancelled')),
		ticket_id TEXT REFERENCES tickets(ticket_id) ON DELETE SET NULL,
		remaining_capacity INTEGER NOT NULL DEFAULT 0 CHECK (remaining_capacity >= 0),
		message TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		completed_at TIMESTAMPTZ
	);

ALTER TABLE booking_idempotency_results DROP CONSTRAINT IF EXISTS booking_idempotency_results_registration_status_check;

ALTER TABLE booking_idempotency_results ADD CONSTRAINT booking_idempotency_results_registration_status_check
		CHECK (registration_status IN ('', 'received', 'confirmed', 'waitlisted', 'cancelled'));

CREATE INDEX IF NOT EXISTS idx_booking_idempotency_results_registration ON booking_idempotency_results(registration_id);

ALTER TABLE tickets DROP CONSTRAINT IF EXISTS tickets_status_check;

ALTER TABLE tickets ADD CONSTRAINT tickets_status_check CHECK (status IN ('active', 'redeemed', 'revoked', 'expired'));

ALTER TABLE tickets ADD COLUMN IF NOT EXISTS sequence_number INTEGER NOT NULL DEFAULT 1;

ALTER TABLE tickets ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

ALTER TABLE tickets ADD COLUMN IF NOT EXISTS revoked_reason TEXT NOT NULL DEFAULT '';

ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0;

ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS schema_version INTEGER NOT NULL DEFAULT 1;

ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS partition_key TEXT;

ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0;

ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '';

ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS dead_letter_at TIMESTAMPTZ;

ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS available_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS lease_started_at TIMESTAMPTZ;

ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS outbox_events_idem_key_idx
		ON outbox_events (event_type, idempotency_key)
		WHERE idempotency_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_outbox_published_lag ON outbox_events(event_type, published_at, created_at)
		WHERE publish_status = 'published' AND published_at IS NOT NULL;

ALTER TABLE offline_checkin_batches ADD COLUMN IF NOT EXISTS valid_until TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '4 hours');

ALTER TABLE offline_checkin_batches ADD COLUMN IF NOT EXISTS package_signature TEXT NOT NULL DEFAULT '';

ALTER TABLE offline_checkin_scans ALTER COLUMN ticket_id DROP NOT NULL;

ALTER TABLE offline_checkin_scans ADD COLUMN IF NOT EXISTS token_hash TEXT NOT NULL DEFAULT '';

ALTER TABLE offline_checkin_scans ADD COLUMN IF NOT EXISTS conflict_reason TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_offline_checkin_scans_batch_token_time
		ON offline_checkin_scans(batch_id, token_hash, scanned_at, created_at);

ALTER TABLE tickets ALTER COLUMN signed_token SET DEFAULT '';

CREATE TABLE IF NOT EXISTS checkin_rejections (
		rejection_id TEXT PRIMARY KEY,
		ticket_id TEXT REFERENCES tickets(ticket_id) ON DELETE SET NULL,
		event_id TEXT NOT NULL DEFAULT '',
		staff_id TEXT NOT NULL,
		device_id TEXT NOT NULL,
		reason TEXT NOT NULL,
		detail TEXT NOT NULL DEFAULT '',
		rejected_at TIMESTAMPTZ NOT NULL DEFAULT now()
	);

CREATE INDEX IF NOT EXISTS idx_checkin_rejections_ticket ON checkin_rejections(ticket_id);

ALTER TABLE tickets ALTER COLUMN qr_payload SET DEFAULT '';

CREATE TABLE IF NOT EXISTS reporting_event_summary (
		event_id             TEXT        NOT NULL PRIMARY KEY,
		total_capacity       INTEGER     NOT NULL DEFAULT 0,
		confirmed_count      INTEGER     NOT NULL DEFAULT 0,
		cancelled_count      INTEGER     NOT NULL DEFAULT 0,
		waitlist_count       INTEGER     NOT NULL DEFAULT 0,
		department_breakdown JSONB       NOT NULL DEFAULT '{}',
		last_processed_at    TIMESTAMPTZ NOT NULL DEFAULT '-infinity',
		updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
		CONSTRAINT reporting_event_summary_confirmed_count_check  CHECK (confirmed_count  >= 0),
		CONSTRAINT reporting_event_summary_cancelled_count_check  CHECK (cancelled_count  >= 0),
		CONSTRAINT reporting_event_summary_waitlist_count_check   CHECK (waitlist_count   >= 0),
		CONSTRAINT reporting_event_summary_total_capacity_check   CHECK (total_capacity   >= 0)
	);

CREATE INDEX IF NOT EXISTS idx_reporting_event_summary_updated_at
		ON reporting_event_summary (updated_at DESC);

CREATE TABLE IF NOT EXISTS reporting_projection_offsets (
		projection_name  TEXT        NOT NULL PRIMARY KEY,
		last_processed_at TIMESTAMPTZ NOT NULL DEFAULT '-infinity',
		updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
	);

INSERT INTO reporting_projection_offsets (projection_name, last_processed_at)
		VALUES ('event_summary', '-infinity')
		ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS booking_bans (
		ban_id TEXT NOT NULL PRIMARY KEY,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		employee_id TEXT NOT NULL REFERENCES employees(employee_id),
		registration_id TEXT NOT NULL REFERENCES registrations(registration_id) ON DELETE CASCADE,
		reason TEXT NOT NULL DEFAULT '',
		banned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		lifted_at TIMESTAMPTZ,
		lifted_by TEXT
	);

CREATE UNIQUE INDEX IF NOT EXISTS booking_bans_unique_active
		ON booking_bans (event_id, employee_id)
		WHERE lifted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_booking_bans_employee
		ON booking_bans (employee_id);

-- PH2-23 reservation TTL & compensation: the worker reconciles orphan Redis
-- pending holds by looking up the corresponding booking via (event_id,
-- idempotency_hash). The hash is HMAC-derived (raw key never appears here);
-- column is nullable because Phase 1 bookings (gate=off) never produce a hash
-- and must continue to insert idempotency rows.
ALTER TABLE booking_idempotency_results ADD COLUMN IF NOT EXISTS idempotency_hash TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_booking_idempotency_results_event_hash
		ON booking_idempotency_results (event_id, idempotency_hash)
		WHERE idempotency_hash IS NOT NULL;

-- PH2-23 compensation metrics sink. The compensation sweep runs in the worker
-- process (never scraped by Prometheus), so each action/result and counter
-- drift result is accumulated as a monotonic total here. The serve /metrics
-- endpoint derives cets_reservation_compensation_total{action,result} and
-- cets_reservation_counter_drift_total{result} from these rows, matching the
-- existing DB-derived worker-outcome/outbox metric pattern. Labels are
-- low-cardinality operational outcomes only — no PII, no idempotency keys.
CREATE TABLE IF NOT EXISTS reservation_compensation_metrics (
		metric TEXT NOT NULL,
		action TEXT NOT NULL DEFAULT '',
		result TEXT NOT NULL,
		total BIGINT NOT NULL DEFAULT 0,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		PRIMARY KEY (metric, action, result)
	);

-- PH2-42 last_event_offset: stores the outbox_id of the last applied event
-- per row in reporting_event_summary for idempotent upsert (older replays skipped)
ALTER TABLE reporting_event_summary ADD COLUMN IF NOT EXISTS last_event_offset TEXT NOT NULL DEFAULT '';

-- PH2-42 last_processed_outbox_id: crash-recovery watermark in
-- reporting_projection_offsets to resume processing after a worker restart
ALTER TABLE reporting_projection_offsets ADD COLUMN IF NOT EXISTS last_processed_outbox_id TEXT NOT NULL DEFAULT '';

-- PH2-45 export aggregate columns: maintained incrementally where the event
-- stream allows, authoritatively by rebuild (cets admin rebuild-projection).
-- Aggregate counts only — no PII.
ALTER TABLE reporting_event_summary ADD COLUMN IF NOT EXISTS employee_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE reporting_event_summary ADD COLUMN IF NOT EXISTS family_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE reporting_event_summary ADD COLUMN IF NOT EXISTS ticket_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE reporting_event_summary ADD COLUMN IF NOT EXISTS checkin_count INTEGER NOT NULL DEFAULT 0;

-- Per-employee hidden events. A personal calendar view preference: a hidden
-- event is dropped from the employee's calendar (to reduce clutter) but stays
-- visible in the event list flagged as hidden. Not booking state, no audit.
-- INSERT is naturally idempotent via the composite primary key.
CREATE TABLE IF NOT EXISTS hidden_events (
		employee_id TEXT NOT NULL REFERENCES employees(employee_id) ON DELETE CASCADE,
		event_id TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
		hidden_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		PRIMARY KEY (employee_id, event_id)
	);

CREATE INDEX IF NOT EXISTS idx_hidden_events_employee ON hidden_events(employee_id);

-- Cancellation idempotency backstop: one cancel_idempotency_key may settle at
-- most one registration. The application-level replay check only compares the
-- key on an already-cancelled registration, so without this index the same
-- key reused against a different registration would cancel it too. Mirrors
-- the booking-side registrations.idempotency_key UNIQUE guarantee.
CREATE UNIQUE INDEX IF NOT EXISTS registrations_unique_cancel_idempotency_key
		ON registrations (cancel_idempotency_key)
		WHERE cancel_idempotency_key IS NOT NULL AND cancel_idempotency_key <> '';
